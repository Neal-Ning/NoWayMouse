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

        # States: 
        # 0: Overlay hidden
        # 1: Overlay division 0
        # 2: Overlay divission 1
        self.state = 0
        self.selectedDiv0Col = 0 # The selected column of division 0
        self.selectedDiv0Row = 0 # The selected row of division 0

        self.SOCKET_PATH = "/tmp/overlay.sock"
        self.HOME = os.path.expanduser("~")
        self.CONFIG_FOLDER = self.HOME + "/.config/nowaymouse/"
        self.DEFAULT_CONFIG_PATH = self.CONFIG_FOLDER + "config.yaml"
        self.USER_CONFIG_PATH = self.CONFIG_FOLDER + "usrconfig.yaml"

        # Mapps field names defined in config to corrresponding variable names in code
        self.config_map = {
            'screen_x_resolution': 'width',
            'screen_y_resolution': 'height',
            'div_1_n_cols': 'div1Cols',
            'div_1_n_rows': 'div1Rows',
            'div_0_first_key': 'div0key0',
            'div_0_second_key_0': 'div0key1R',
            'div_0_second_key_1': 'div0key1L',
            'div_1_key': 'div1keys'
        }

        # Init layer shell
        GtkLayerShell.init_for_window(self) # Use wayland layer shell protocol
        GtkLayerShell.set_layer(self, GtkLayerShell.Layer.OVERLAY)
        GtkLayerShell.set_anchor(self, Gtk.PositionType.LEFT, True)
        GtkLayerShell.set_anchor(self, Gtk.PositionType.RIGHT, True)
        GtkLayerShell.set_anchor(self, Gtk.PositionType.TOP, True)
        GtkLayerShell.set_anchor(self, Gtk.PositionType.BOTTOM, True)

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

        # Load default, user overwrites and inferred variables
        self.load_config(self.DEFAULT_CONFIG_PATH)
        self.load_config(self.USER_CONFIG_PATH)
        self.finalize_config()

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
        with open(config, 'r') as file:
            data = yaml.safe_load(file) or {}

        for config_key, var_name in self.config_map.items():
            if config_key in data:
                setattr(self, var_name, data[config_key])

    # Infer more variables from already defined variables
    def finalize_config(self):
        self.div0Cols = len(self.div0key0)
        self.div0Rows = len(self.div0key1R)
        self.box0X = int(self.width / self.div0Cols)
        self.box0Y = int(self.height / self.div0Rows)
        self.box1X = int(self.box0X / self.div1Cols)
        self.box1Y = int(self.box0Y / self.div1Rows)

    # Listens for messages from the main go code
    def socket_listener(self):
        try:
            os.unlink(self.SOCKET_PATH)
        except FileNotFoundError:
            print("Sock file not found at ", self.SOCKET_PATH)

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
                        self.state = 0
                        GLib.idle_add(self.hide)
                    # Show overlay with division 0, state = 1
                    elif self.overlay_request[0] == "show0":
                        self.state = 1
                        GLib.idle_add(self.show_all)
                    # Show overlay with division 1, state = 2
                    elif self.overlay_request[0] == "show1":
                        # Note: hide again to reload the overlay
                        GLib.idle_add(self.hide)
                        self.state = 2
                        self.selectedDiv0Col = int(self.overlay_request[1])
                        self.selectedDiv0Row = int(self.overlay_request[2])
                        GLib.idle_add(self.show_all)
                        GLib.idle_add(self.area.queue_draw)

    # Called everytime GTK redraws the overlay
    def on_draw(self, widget, cr):

        if self.state == 0: return

        if self.state == 1:

            cr.set_operator(cairo.OPERATOR_CLEAR)
            cr.paint()
            cr.set_operator(cairo.OPERATOR_OVER)

            # Color background (black) and set transparency
            cr.set_source_rgba(0, 0, 0, 0.3)
            cr.rectangle(0, 0, self.width, self.height)
            cr.fill()

            # Draw division lines
            cr.set_source_rgba(1, 1, 1, 0.3)
            cr.set_line_width(2)
            cr.rectangle(0, 0, self.width, self.height)
            cr.rectangle(self.box0X, 0, 0, self.height)
            for i in range(self.div0Cols):
                cr.rectangle(self.box0X * i, 0, 0, self.height)
            for i in range(self.div0Rows):
                cr.rectangle(0, self.box0Y * i, self.width, 0)
            cr.stroke()

            # Draw text
            cr.set_source_rgba(1, 1, 1, 0.8)
            cr.select_font_face("Sans", cairo.FONT_SLANT_NORMAL, cairo.FONT_WEIGHT_BOLD)
            cr.set_font_size(32)

            for i in range(self.div0Cols):
                for j in range(self.div0Rows):
                    cr.move_to(self.box0X * i + int(self.box0X / 3), self.box0Y * j + int(self.box0Y / 2) + 20)
                    if i < 5:
                        cr.show_text(self.div0key0[i] + self.div0key1R[j])
                    else:
                        cr.show_text(self.div0key0[i] + self.div0key1L[j])

        elif self.state == 2:

            cr.set_operator(cairo.OPERATOR_CLEAR)
            cr.paint()
            cr.set_operator(cairo.OPERATOR_OVER)

            # Color background (black) and set transparency
            cr.set_source_rgba(0, 0, 0, 0.3)
            cr.rectangle(0, 0, self.width, self.height)
            cr.fill()

            # Draw division lines
            cr.set_source_rgba(1, 1, 1, 0.3)
            cr.set_line_width(2)
            startCoordX = self.selectedDiv0Col * self.box0X
            startCoordY = self.selectedDiv0Row * self.box0Y
            cr.rectangle(startCoordX, startCoordY, self.box0X, self.box0Y)
            for i in range(self.div1Cols):
                cr.rectangle(startCoordX + self.box1X * i, startCoordY, 0, self.box0Y)
            for i in range(self.div1Rows):
                cr.rectangle(startCoordX, startCoordY + self.box1Y * i, self.box0X, 0)
            cr.stroke()

            # Draw text
            cr.set_source_rgba(1, 1, 1, 0.8)
            cr.select_font_face("Sans", cairo.FONT_SLANT_NORMAL, cairo.FONT_WEIGHT_BOLD)
            cr.set_font_size(16)

            for i in range(self.div1Cols):
                for j in range(self.div1Rows):
                    cr.move_to(startCoordX + self.box1X * i + int(self.box1X / 3), startCoordY + self.box1Y * j + int(self.box1Y / 2) + 7)
                    cr.show_text(self.div1keys[j * self.div1Cols + i])


if __name__ == "__main__":
    win = OverlayWindow()
    Gtk.main()
