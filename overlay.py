import gi
import cairo
import os
import socket
import threading
import yaml

gi.require_version("Gtk", "3.0")
gi.require_version("GtkLayerShell", "0.1")
from gi.repository import Gtk, Gdk, GtkLayerShell, GLib

# Class to draw overlay over screen
class OverlayWindow(Gtk.Window):
    def __init__(self):
        super().__init__()

        # ========== Variables ========== #

        # Division counter / tracker
        self.div_count = -1

        # Path variables
        self.SOCKET_PATH = "/tmp/overlay.sock"
        self.DEFAULT_CONFIG_PATH = os.path.join(os.path.dirname(os.path.abspath(__file__)), "default.yaml")
        self.USER_CONFIG_PATH = os.path.join(os.path.expanduser("~"), ".config", "nowaymouse", "config.yaml")

        # Mapps field names defined in config to corrresponding variable names in code
        self.config_map = {
            'screen_x_resolution': 'width',
            'screen_y_resolution': 'height',
            'number_of_divisions': 'n_divs',
            'division_dimensions': 'div_dim',
            'division_navigators': 'div_keys',
            'overlay_rgba' : 'overlay_rgba',
            'division_lines_rgba': 'div_line_rgba',
            'division_navigators_rgba': 'div_key_rgba',
            'font_size_multiplier': 'font_size_multiplier',
        }

        # ========== Config Initializations ========== #

        # Load default, user overwrites and inferred variables
        self.load_config(self.DEFAULT_CONFIG_PATH)
        self.load_config(self.USER_CONFIG_PATH)
        self.finalize_config()

        # ========== GTK Initializations ========== #

        # Init layer shell
        GtkLayerShell.init_for_window(self) # Use wayland layer shell protocol
        GtkLayerShell.set_layer(self, GtkLayerShell.Layer.OVERLAY)
        GtkLayerShell.set_anchor(self, Gtk.PositionType.LEFT, True)
        GtkLayerShell.set_anchor(self, Gtk.PositionType.RIGHT, True)
        GtkLayerShell.set_anchor(self, Gtk.PositionType.TOP, True)
        GtkLayerShell.set_anchor(self, Gtk.PositionType.BOTTOM, True)
        GtkLayerShell.set_exclusive_zone(self, -1) # Cover the whole screen

        # Transparent background
        self.set_app_paintable(True)
        screen = self.get_screen()
        visual = screen.get_rgba_visual()
        if visual is not None and screen.is_composited():
            self.set_visual(visual)

        # Extra settings
        self.set_decorated(False)
        self.set_skip_taskbar_hint(True)
        self.set_skip_pager_hint(True)
        self.set_accept_focus(False)
        self.set_resizable(False)

        # Drawing Area (manual sizing)
        self.area = Gtk.DrawingArea()
        self.area.connect("draw", self.on_draw)
        self.area.set_size_request(self.width, self.height)

        # Use fixed container
        fixed = Gtk.Fixed()
        fixed.put(self.area, 0, 0)
        self.add(fixed)

        # Pre load the rendering
        self.show_all()
        self.area.queue_draw()
        self.hide()

        # Start a thread listening to the socket
        threading.Thread(target=self.socket_listener, daemon=True).start()

    # Load the contents of the config file specified at path
    def load_config(self, config):
        if not os.path.exists(config): return

        with open(config, 'r') as file:
            data = yaml.safe_load(file) or {}

        for config_key, var_name in self.config_map.items():
            if config_key in data:
                setattr(self, var_name, data[config_key])

    # Infer more variables from already defined variables
    def finalize_config(self):
        # Divided area of each division
        self.div_area = [[float(self.width), float(self.height)]]
        for i in range(len(self.div_dim)):
            self.div_area.append([
                float(self.div_area[i][0] / float(self.div_dim[i][0])),
                float(self.div_area[i][1] / float(self.div_dim[i][1]))
            ])

        # Create dummy cairo object to evaluate keys in pixel length
        surface = cairo.ImageSurface(cairo.FORMAT_ARGB32, 1, 1)
        cr = cairo.Context(surface)
        cr.select_font_face("Sans", cairo.FONT_SLANT_NORMAL, cairo.FONT_WEIGHT_BOLD)

        def text_width(text):
            cr.set_font_size(100)
            return cr.text_extents(text).width

        def fit_text_width(text, target_width):
            cr.set_font_size(100)
            width = cr.text_extents(text).width
            scale = (target_width * 100) / width
            return scale

        # Find longest key for each division
        self.longest_key = ["" for _ in self.div_keys]
        for i in range(len(self.div_keys)):
            for key in self.div_keys[i]:
                if text_width(self.longest_key[i]) < text_width(key):
                    self.longest_key[i] = key

        # Define font size for each division
        self.font_sizes = []
        for i in range(len(self.longest_key)):
            self.font_sizes.append(fit_text_width(self.longest_key[i], self.div_area[i+1][0] / (2 - self.font_size_multiplier)))


    # Listens for messages from the main go code
    def socket_listener(self):
        try:
            os.unlink(self.SOCKET_PATH)
        except FileNotFoundError:
            pass

        server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        server.bind(self.SOCKET_PATH)
        server.listen(1)

        while True:
            conn, _ = server.accept()
            with conn:
                data = conn.recv(1024).decode()
                if data:
                    self.overlay_request = data.split(",")
                    # Hide the overlay, State = 0
                    if self.overlay_request[0] == "hide":
                        self.div_count = -1
                        GLib.idle_add(self.hide)
                    elif self.overlay_request[0] == "show":
                        # Note: hide again to reload the overlay
                        GLib.idle_add(self.hide)
                        self.div_count = int(self.overlay_request[1])
                        self.current_div_box_x = float(self.overlay_request[2])
                        self.current_div_box_y = float(self.overlay_request[3])
                        GLib.idle_add(self.show_all)
                        GLib.idle_add(self.area.queue_draw)

    # Called everytime GTK redraws the overlay
    def on_draw(self, widget, cr):

        if self.div_count == -1: return

        cr.set_operator(cairo.OPERATOR_CLEAR)
        cr.paint()
        cr.set_operator(cairo.OPERATOR_OVER)

        # Color background (black) and set transparency
        cr.set_source_rgba(*[val if id == 3 else val / 255 for id, val in enumerate(self.overlay_rgba)])
        cr.rectangle(0, 0, self.width, self.height)
        cr.fill()

        # Draw division lines
        cr.set_source_rgba(*[val if id == 3 else val / 255 for id, val in enumerate(self.div_line_rgba)])
        cr.set_line_width(2)

        # Infer auxilliary variables
        start_coord_x = self.current_div_box_x
        start_coord_y = self.current_div_box_y
        box_width = self.div_area[self.div_count][0]
        box_height = self.div_area[self.div_count][1]
        next_box_width = self.div_area[self.div_count + 1][0]
        next_box_height = self.div_area[self.div_count + 1][1]

        # Draw the division grid
        cr.rectangle(start_coord_x, start_coord_y, box_width, box_height)
        for i in range(1, self.div_dim[self.div_count][0]):
            cr.rectangle(start_coord_x + next_box_width * i, start_coord_y, 0, box_height)
        for i in range(1, self.div_dim[self.div_count][1]):
            cr.rectangle(start_coord_x, start_coord_y + next_box_height * i, box_width, 0)
        cr.stroke()


        # Text configs
        cr.set_source_rgba(*[val if id == 3 else val / 255 for id, val in enumerate(self.div_key_rgba)])
        cr.select_font_face("Sans", cairo.FONT_SLANT_NORMAL, cairo.FONT_WEIGHT_BOLD)
        cr.set_font_size(self.font_sizes[self.div_count])

        # Draw text
        for i in range(self.div_dim[self.div_count][0]):
            for j in range(self.div_dim[self.div_count][1]):
                text = self.div_keys[self.div_count][j * self.div_dim[self.div_count][0] + i]
                box_center_x = start_coord_x + next_box_width * i + int(next_box_width / 2)
                box_center_y = start_coord_y + next_box_height * j + int(next_box_height / 2)

                # Compute coordinate to fit text to center of box
                extents = cr.text_extents(text)
                move_to_x = box_center_x - (extents.width / 2 + extents.x_bearing)
                move_to_y = box_center_y - (extents.height / 2 + extents.y_bearing)

                cr.move_to(move_to_x, move_to_y)
                cr.show_text(text)


if __name__ == "__main__":
    win = OverlayWindow()
    Gtk.main()
