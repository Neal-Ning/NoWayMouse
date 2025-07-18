package main

import (
	"fmt"
	"sync"
	"unicode/utf8"

	"os"
	"os/exec"
	"os/user"
	"path/filepath"

	"github.com/bendahl/uinput"
	"github.com/gvalkov/golang-evdev"
	"gopkg.in/yaml.v3"
)

// =============================== //
// ========== Variables ========== //
// =============================== //

var (
	// Config loader
	config Config

	// ========== Path variables ========== //

	socketPath string = "/tmp/overlay.sock"
	defaultConfigPath string = getScriptPath("default.yaml")
	userConfigPath string = filepath.Join("/home", userName(), ".config", "nowaymouse", "config.yaml")

	// ========== Configuratble variables ========== //

	// The keyboard input device: /dev/input/eventn
	keyboardPath string

	// Mouse mode keybinds
	activationKey string
	divKey string
	mouseClick string
	mouseUp string
	mouseLeft string
	mouseDown string
	mouseRight string
	scrollUp string
	scrollLeft string
	scrollDown string
	scrollRight string
	mouseSpeed int32
	scrollSpeed int32

	// Resolution
	resX int32
	resY int32

	// Number of divisions
	nDivs int

	// Dimension of the divisions
	divDim [][]int32

	// Keys used to navigate the divisions
	divKeys [][]string

	// ========== Inferrable variables ========== //

	// Mappings from defined keys to their index
	divKeyMaps []map[string]int32
	
	// Dimensions of each box in the divisions
	divArea [][2]float32

	// Longest navigator keys for a division
	longestKeyLen []int

	// ========== Software and application variables ========== //

	// Simulation / Program variables
	keyInput *evdev.InputDevice // Registered keyboard
	keyboard uinput.Keyboard // Virtual keyboard
	mouse uinput.Mouse // Virtual mouse
	overlayProc *exec.Cmd // Overlay python process

	// System environment variables
	display string = os.Getenv("DISPLAY")
	wayland string = os.Getenv("WAYLAND_DISPLAY")
	runtimeDir string = os.Getenv("XDG_RUNTIME_DIR")
	dbus string = ("DBUS_SESSION_BUS_ADDRESS")

	// State variables
	adjustMode int = 2
	mouseMode bool = false // Keyboard controls mouse
	divMode bool = false // Overlay to divide screen into boxes
	divCount int = -1 // Number of divisions currently done
	currentDivBoxX float32 = 0 // X of corner of the box selected from the last division
	currentDivBoxY float32 = 0 // Y of corner of the box selected from the last division
	pressed string = ""// String of pressed (navigation) keys during the current division

	// Other
	heldMovementKeys map[string]bool
	movementMutex = sync.RWMutex{}
)

// ============================ //
// ========== Config ========== //
// ============================ //


// Config variable mapping
type Config struct {
	KeyboardPath string `yaml:"keyboard_input_path"`
	ActivationKey string `yaml:"activation_key"`
	DivKey string `yaml:"activate_division_overlay_key"`
	MouseClick string `yaml:"mouse_click"`
	MouseUKey string `yaml:"mouse_up"`
	MouseLKey string `yaml:"mouse_left"`
	MouseDKey string `yaml:"mouse_down"`
	MouseRKey string `yaml:"mouse_right"`
	ScrollUKey string `yaml:"scroll_up"`
	ScrollLKey string `yaml:"scroll_left"`
	ScrollDKey string `yaml:"scroll_down"`
	ScrollRKey  string `yaml:"scroll_right"`
	MouseSpeed int32 `yaml:"mouse_speed"`
	ScrollSpeed int32 `yaml:"scroll_speed"`
	ResX int32 `yaml:"screen_x_resolution"`
	ResY int32 `yaml:"screen_y_resolution"`
	NDivs int `yaml:"number_of_divisions"`
	DivDim [][]int32 `yaml:"division_dimensions"`
	DivKeys	[][]string `yaml:"division_navigators"`
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
	divKey = config.DivKey
	mouseClick = config.MouseClick
	mouseUp = config.MouseUKey
	mouseLeft = config.MouseLKey
	mouseDown = config.MouseDKey
	mouseRight = config.MouseRKey
	scrollUp = config.ScrollUKey
	scrollLeft = config.ScrollLKey
	scrollDown = config.ScrollDKey
	scrollRight = config.ScrollRKey
	mouseSpeed = config.MouseSpeed
	scrollSpeed = config.ScrollSpeed
	resX = config.ResX
	resY = config.ResY
	nDivs = config.NDivs
	divDim = config.DivDim
	divKeys = config.DivKeys
}

// Verify that the defined config is sound
func verify_config() {
	if (nDivs != len(divDim)) {
		panic(fmt.Sprintf("Error in config: \nnumber_of_divisions = %v, but defined %v sets of division_dimensions.\n", nDivs, len(divDim)))
	}
	if (nDivs != len(divKeys)) {
		panic(fmt.Sprintf("Error in config: \nnumber_of_divisions = %v, but defined %v sets of division_navigators.\n", nDivs, len(divKeys)))
	}

	var boxSizeX = float32(resX)
	var boxSizeY = float32(resY)
	for i, dim := range divDim {
		if (len(dim) != 2 || dim[0] < 1 || dim[1] < 1) {
			panic(fmt.Sprintf("Error in config: \nDivision dimension: %v not in format: [x, y], where x, y >= 1.\n", dim))
		}

		if (len(divKeys[i]) != int(dim[0] * dim[1])) {
			panic(fmt.Sprintf("Error in config: \n%v keys defined for division %v. \nThis doesn't fit or cover the defined dimension of %v * %v\n", len(divKeys[i]), i, dim[0], dim[1]))
		}

		boxSizeX /= float32(dim[0])
		boxSizeY /= float32(dim[1])
	}
	if (boxSizeX < 5 || boxSizeY < 5) {
		panic("Error in config: \nYou have divided too much. The last division should at least have 5x5 grids.\n")
	}
}

// Finalize config by inferring some necessary variables
func finalize_config() {

	// Create mapping of keys to their index for search up
	divKeyMaps = make([]map[string]int32, nDivs)
	longestKeyLen = make([]int, nDivs)
	for i, keys := range divKeys {
		divKeyMaps[i] = make(map[string]int32)
		for j, key := range keys {
			divKeyMaps[i][key] = int32(j)
			// Record the longest navigation key for each division
			var keyRuneCount int = utf8.RuneCountInString(key)
			if longestKeyLen[i] < keyRuneCount {
				longestKeyLen[i] = keyRuneCount
			}
		}
	}

	// Divided area of each division
	divArea = make([][2]float32, nDivs+1)
	divArea[0] = [2]float32{float32(resX), float32(resY)}
	for i, dim := range divDim {
		divArea[i + 1][0] = divArea[i][0] / float32(dim[0])
		divArea[i + 1][1] = divArea[i][1] / float32(dim[1])
	}

	// Define mapping of movement keys to their held state
	heldMovementKeys = map[string]bool{
		mouseUp: false,
		mouseLeft: false,
		mouseDown: false,
		mouseRight: false,
		scrollUp: false,
		scrollLeft: false,
		scrollDown: false,
		scrollRight: false,
	}
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
	}
}

// Create a virtual mouse controller to move the mouse around
func initMouse() {
	var err error
	mouse, err = uinput.CreateMouse("/dev/uinput", []byte("virtual-mouse"))
	if err != nil {
		fmt.Printf("Failed to create virtual mouse: %v \n", err)
	}
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
