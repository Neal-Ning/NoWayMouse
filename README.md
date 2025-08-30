# NoWayMouse

**nowaymouse** provides effective keyboard-based control of mouse position, movements and actions on **wayland based compositors**.

## Show case
### Mouse mode
When you activate **Mouse Mode**, you can move the mouse pointer around, as well as click and scroll.

[Insert gif]

### Div mode
When you activate **Division Mode**, the screen is divided into a grid, and you can precisely choose where the mouse pointer should jump through multiple divisions.

[Insert gif2]

## Config

The default config (all configrable variables) is available at `nowaymouse/default.yaml`.
To customize settings, create a configuration file at `~/.config/nowaymouse/config.yaml`.

> Important: You must override the keyboard_input_path variable in your configuration. Without this, nowaymouse will not know which device to listen to for keyboard input.

You may also override any other variables by defining them in `~/.config/nowaymouse/config.yaml`.

---

### Required step: Keyboard Path Configuration
The application needs to know which input device corresponds to your keyboard. To set this up:
1. Install evtest (via `apt`, `pacman`, or any package manager of choice)
2. Run:
    ```
    sudo evtest
    ```
3. Identify the path of your keyboard device from the listed input devices
    - If multiple keyboards are listed, select each by number and press keys to see if they register.
4. Append the following line to your `~/.config/nowaymouse/config.yaml`:  
   ```yaml
   keyboard_input_path: /dev/input/eventX
    ```
---

### Activation key keybinds
**Mouse Mode** is activated by pressing the key defined in the `activation_key` variable. 
- In Mouse Mode, you can move the pointer, click and scroll with the defined keys.
- Note: Once nowaymouse is running, the chosen activationkey will not perform its original function.

**Div(Division) Mode** is activated by pressing the key defined in the `activation_division_overlay_key` variable.
- You must first enter Mouse Mode before activating Div Mode.
- A future update may allow Div Mode to be activated directly without being in Mouse Mode.

---

### Overlay config
The default behavior defined in `nowaymouse/default.yaml` entails: When you press the `activation_division_overlay_key`, an overlay grid covers the screen, where each grid cell contains a unique combination of letters called **navigators**.
- Typing out a navigator sequence splits the corresponding grid into smaller grid cells;
- Each smaller grid cell is labeled with a single-letter navigator;
- Then typing a letter moves the mouse pointer the center of the corresponding smaller grid cell. 
- The mouse automatically left click once at its new position, and exits Div Mode.

#### Configrable options:
- `screen_x_resolution`, `screen_y_resolution`: -> Define yoru screen resolution;
- `number_of_divisions` -> Define the depth of recursive grid splitting;
- `division_dimensions` -> Define the grid size (columns, rows) at each division;
- `division_navigators` -> Define the set of characters used as navigators;
- `click_after_select` -> Whether to automatically click after selecting the final grid;
- `mouse_mode_after_select` -> Whether to stay in Mouse Mode after selection. 
> Note: Ensure `number_of_divisions`, `division_dimensions`, and `division_navigators` are consistent.

---

## Extra Notes
Currently, nowaymouse uses keycode for an English keybaord layout. If you use a different keyboard layout, some keys (especially punctuations, symbols, and special characters) may not match correctly.
