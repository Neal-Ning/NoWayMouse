package main

import (
	"fmt"
	"time"

	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"

	"github.com/bendahl/uinput"
	"github.com/gvalkov/golang-evdev"
	"gopkg.in/yaml.v3"
)

var (
	// Config
	config Config

	// ========== Path variables ========== //

	socketPath string = "/tmp/overlay.sock"
	defaultConfigPath string = filepath.Join("/home", userName(), ".config", "nowaymouse", "config.yaml")
	userConfigPath string = filepath.Join("/home", userName(), ".config", "nowaymouse", "usrconfig.yaml")

	// ========== Configuratble variables ========== //

	// The keyboard input device: /dev/input/eventn
	keyboardPath string

	// Keybinds
	activationKey string
	overlayKey string

	// Resolution
	resX int32
	resY int32

	// Number of cols and rows in the second division
	div1Cols int32
	div1Rows int32

	// Keys used to navigate the divisions
	div0XKeys []string
	div0Y0Keys []string
	div0Y1Keys []string
	div1Keys []string

	// ========== Inferrable variables ========== //

	// Mappings from defined keys to their index
	div0XKeyMap map[string]int32
	div0Y0KeyMap map[string]int32
	div0Y1KeyMap map[string]int32
	div1KeyMap map[string]int32
	
	// Dimension of the first division
	div0Cols int32
	div0Rows int32

	// Dimensions of each box in the first division
	box0X int32
	box0Y int32

	// Dimensions of each box in the second division
	box1X int32
	box1Y int32

	// ========== Software and application variables ========== //

	// Simulation / Program variables
	keyInput *evdev.InputDevice // Registered keyboard
	keyboard uinput.Keyboard // Virtual keyboard
	mouse uinput.Mouse // Virtual mouse
	overlayProc *exec.Cmd // Overlay python process

	// System environment variables
	waybarHeight int32 = 30 // Height of the waybar
	display string = os.Getenv("DISPLAY")
	wayland string = os.Getenv("WAYLAND_DISPLAY")
	runtimeDir string = os.Getenv("XDG_RUNTIME_DIR")
	dbus string = ("DBUS_SESSION_BUS_ADDRESS")

	// State variables
	adjustMode int = 2
	mouseMode bool = false // Keyboard controls mouse
	overlayMode bool = false // Overlay to divide screen into boxes
	overlay01 bool = false // Overlay showing the first divide, 1 input given
	overlay02 bool = false // Overlay showing the first divide, 2 inputs given
	overlay1 bool = false // Overlay showing the second divide, 1 input given

	// Temporary state variables
	selectedDiv0Col int32
	selectedDiv0Row int32
)

// Config variable mapping
type Config struct {
	KeyboardPath string `yaml:"keyboard_input_path"`
	ActivationKey string `yaml:"activation_key"`
	OverlayKey string `yaml:"overlay_key"`
	ResX int32 `yaml:"screen_x_resolution"`
	ResY int32 `yaml:"screen_y_resolution"`
	Div1Cols int32 `yaml:"div_1_n_cols"`
	Div1Rows int32 `yaml:"div_1_n_rows"`
	Div0XKeys []string `yaml:"div_0_first_key"`
	Div0Y0Keys []string `yaml:"div_0_second_key_0"`
	Div0Y1Keys []string `yaml:"div_0_second_key_1"`
	Div1Keys []string `yaml:"div_1_key"`
}

// Get name of current user
func userName() string {
	sudoUser := os.Getenv("SUDO_USER")
	if (sudoUser != "") {
		usr, err := user.Lookup(sudoUser)
		if err == nil {
			return usr.Username
		}
		fmt.Printf("Failed to lookup SUDO_USER (%s): %v \n", sudoUser, err)
	}
	return ""
}

// Function that loads the config
func load_config(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("Read config error: ", err)
	}
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		fmt.Println("Parse config error: ", err)
	}
}

// Define variables after loading default and user config
func set_config() {
	keyboardPath = config.KeyboardPath
	activationKey = config.ActivationKey
	overlayKey = config.OverlayKey
	resX = config.ResX
	resY = config.ResY
	div1Cols = config.Div1Cols
	div1Rows = config.Div1Rows
	div0XKeys = config.Div0XKeys
	div0Y0Keys = config.Div0Y0Keys
	div0Y1Keys = config.Div0Y1Keys
	div1Keys = config.Div1Keys
}

// Finalize config by inferring some necessary variables
func finalize_config() {

	// Create mapping of keys to their index for search up
	div0XKeyMap = make(map[string]int32)
	for i, v := range div0XKeys {
		div0XKeyMap[v] = int32(i)
	}

	div0Y0KeyMap = make(map[string]int32)
	for i, v := range div0Y0Keys {
		div0Y0KeyMap[v] = int32(i)
	}

	div0Y1KeyMap = make(map[string]int32)
	for i, v := range div0Y1Keys {
		div0Y1KeyMap[v] = int32(i)
	}

	div1KeyMap = make(map[string]int32)
	for i, v := range div1Keys {
		div1KeyMap[v] = int32(i)
	}
	
	// Number of columns and rows in the first division
	div0Cols = int32(len(div0XKeyMap))
	div0Rows = int32(len(div0Y0KeyMap))

	// Dimensions of each box in the first division
	box0X = resX / div0Cols
	box0Y = resY / div0Rows

	// Dimensions of each box in the second division
	box1X = box0X / div1Cols
	box1Y = box0Y / div1Rows
}

// Open the keyboard located at keyboardPath to read inputs from it
// Create a virtual keyboard so that some key presses can be filtered
func initKeyboard() {
	var err error
	keyInput, err = evdev.Open(keyboardPath)
	if err != nil {
		fmt.Printf("Failed to open device: %v \n", err)
	} else {
		fmt.Printf("Opened keyboard device: %s\n", keyInput.Name)
	}

	keyboard, err = uinput.CreateKeyboard("/dev/uinput", []byte("virtual-keyboard"))
	if err != nil {
		fmt.Printf("Failed to create virtual keyboard: %v \n", err)
	} else {
		fmt.Printf("Created virtual keyboard")
	}
}

// Create a virtual mouse controller to move the mouse around
func initMouse() {
	var err error
	mouse, err = uinput.CreateMouse("/dev/uinput", []byte("virtual-mouse"))
	if err != nil {
		fmt.Printf("Failed to create virtual mouse: %v \n", err)
	} else {
		fmt.Printf("Created virtual mouse")
	}
}

// Get the folder contianing this script, and append python script name to it
func getScriptPath(scriptName string) string {
	exePath, err := os.Executable()
	if err != nil {
		fmt.Println("Failed to get executable path:", err)
		os.Exit(1)
	}
	exeDir := filepath.Dir(exePath)
	return filepath.Join(exeDir, scriptName)
}

// Start overlay python process with non-root environtment
func initOverlay() {

	pythonPath := getScriptPath(filepath.Join(".venv", "bin", "python"))
    scriptPath := getScriptPath("overlay.py")

    fullCmd := fmt.Sprintf("DISPLAY=%s WAYLAND_DISPLAY=%s XDG_RUNTIME_DIR=%s DBUS_SESSION_BUS_ADDRESS='%s' %s %s",
        display, wayland, runtimeDir, dbus, pythonPath, scriptPath)

	// Run command as non-root user with required environment variables
    overlayProc := exec.Command("runuser", "-l", userName(), "-c", fullCmd)

	// Display prints and errors from running the command
	overlayProc.Stdout = os.Stdout
	overlayProc.Stderr = os.Stderr

	err := overlayProc.Start()
	if err != nil {
		fmt.Printf("Failed to start overlay: %v \n", err)
		return
	}
}

// Move the mouse to an absolute position
func mouseAbs(x int32, y int32) {
	// Move the mouse to upper left corner
	mouse.Move(-resX - 100, -resY - 100)
	// Move the mouse to defined position, with consideration to
	// the fact that overlay.py does not cover the way bar
	mouse.Move(x, y * (resY - waybarHeight) / resY + waybarHeight)
}

// Kill the overlay process
func terminateOverLay() {
	if overlayProc != nil && overlayProc.Process != nil {
		_ = overlayProc.Process.Kill()
	}
	exec.Command("pkill", "-f", "overlay.py").Run()
}

// Display the overlay showing the first split
func showOverlay0() {
	messageOverlay([]byte("show0,100,100"))
}

// Display the overlay showing the second split
// Parameters indicate which box was selected in the first split
func showOverlay1(boxX int32, boxY int32) {
	messageOverlay([]byte(fmt.Sprintf("show1,%v,%v", boxX, boxY)))
}

// Hide the overlay
func hideOverlay() {
	messageOverlay([]byte("hide,100,100"))
}

// Send message to overlay python process to change the overlay display
func messageOverlay(msg []byte) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		fmt.Println("Failed to connect to overlay socket: ", err)
		return
	}
	defer conn.Close()
	_, err = conn.Write(msg)
	if err != nil {
		fmt.Println("Failed to send to overlay socket: ", err)
	}
}

// Exit overlay mode and hide overlay
func exitOverlayMode() {
	overlayMode = false
	hideOverlay()
}

// Enter ovelay mode
func enterOverlayMode() {
	overlayMode = true
}

// Main loop
func main() {

	// Load configs
	load_config(defaultConfigPath)
	load_config(userConfigPath)
	set_config()
	finalize_config()

	// Initializations
	initKeyboard()
	defer keyboard.Close()
	keyInput.Grab()
	defer keyInput.Release()
	initMouse()
	defer mouse.Close()
	initOverlay()
	defer terminateOverLay()

	mainLoop:
	for {
		events, err := keyInput.Read()
		if err != nil {
			fmt.Printf("Read error: %v \n", err)
			continue
		}

		for _, event := range events {
			if (event.Type == evdev.EV_KEY) {

				// Activation key press to enter or exit mouse mode
				if (event.Value == 1 && keyNames[event.Code] == activationKey) {
					if (mouseMode) {
						mouseMode = false
						hideOverlay()
					} else {
						mouseMode = true
					}
					continue mainLoop
				}

				// Pass all captured key presses in normal mode
				if (!mouseMode && !overlayMode) {
					switch event.Value {
					case 1:
						keyboard.KeyDown(int(event.Code))
					case 0:
						keyboard.KeyUp(int(event.Code))
					case 2:
						// keyboard.KeyDown(int(event.Code))
						continue
					}
				}

				// Mousemode and user has pressed key to enter overlay
				if (mouseMode && keyNames[event.Code] == overlayKey && event.Value == 0) {
					mouseMode = false
					enterOverlayMode()
					overlay01 = false
					overlay02 = false
					overlay1 = false
					showOverlay0()
					continue
				}

				// Smoother movement necessary
				if (mouseMode) {
					if (keyNames[event.Code] == "W") {
						mouse.Move(0, -10)
					} else if (keyNames[event.Code] == "A") {
						mouse.Move(-10, 0)
					} else if (keyNames[event.Code] == "S") {
						mouse.Move(0, 10)
					} else if (keyNames[event.Code] == "D") {
						mouse.Move(10, 0)
					} else if (keyNames[event.Code] == "J") {
						mouse.Wheel(false, -5)
					} else if (keyNames[event.Code] == "K") {
						mouse.Wheel(false, 5)
					} else if (keyNames[event.Code] == "H") {
						mouse.Wheel(true, -5)
					} else if (keyNames[event.Code] == "L") {
						mouse.Wheel(true, -5)
					} else if (keyNames[event.Code] == "SPACE") {
						mouse.LeftClick() // Double check
					}
				}

				// User has pressed key in overlay mode
				if (overlayMode && event.Value == 0) {
					// If current keystroke is the first input after overlay mode is activated
					if (!overlay01) {
						selectedDiv0Col = div0XKeyMap[keyNames[event.Code]]
						overlay01 = true

					// If current keystroke is the second input after overlay mode is activated
					} else if (overlay01 && !overlay02) {

						// Calculate coordinates
						if (selectedDiv0Col < div0Cols / 2) {
							selectedDiv0Row = div0Y0KeyMap[keyNames[event.Code]]
						} else {
							selectedDiv0Row = div0Y1KeyMap[keyNames[event.Code]]
						}
						overlay02 = true

						// Calculate the absolute position of the center of the selected box
						var selectedBox0Xabs = selectedDiv0Col * box0X + box0X / 2
						var selectedBox0Yabs = selectedDiv0Row * box0Y + box0Y / 2

						// Move the mouse to the center of the selected box and display second overlay
						mouseAbs(selectedBox0Xabs, selectedBox0Yabs)
						showOverlay1(selectedDiv0Col, selectedDiv0Row)

						if (adjustMode == 1) {
							exitOverlayMode()
							mouseMode = true
						} else if (adjustMode == 2) {
							continue
						}

					// If current keystroke is the third input after overlay mode is activated
					} else if (overlay01 && overlay02 && !overlay1) {
						var selectedBox1 int32 = div1KeyMap[keyNames[event.Code]]

						// Calculate the absolute position of the center of the selected box
						var selectedBox1Xabs = selectedDiv0Col * box0X + selectedBox1 % div1Cols * box1X + box1X / 2
						var selectedBox1Yabs = selectedDiv0Row * box0Y + selectedBox1 / div1Cols * box1Y + box1Y / 2

						// Move the mouse to the center of the selected box and exit overlay
						mouseAbs(selectedBox1Xabs, selectedBox1Yabs)
						exitOverlayMode()

						// Wait for overlay to exit
						time.Sleep(10 * time.Millisecond)
						mouse.LeftClick()
					}
				}
			}
		}
	}
}
