package main

import (
	"fmt"
	"time"

	"net"
	"os/exec"

	"github.com/gvalkov/golang-evdev"
)

// Move the mouse to an absolute position
func mouseAbs(x int32, y int32) {
	// Move the mouse to upper left corner
	mouse.Move(-resX - 100, -resY - 100)
	// Move the mouse to defined position, with consideration to
	mouse.Move(x, y)	
}

// Separate process to monitor held movement keys
func movementLoop() {
	for {
		movementMutex.RLock()
		if (heldMovementKeys[mouseUp]) {
			mouse.Move(0, -mouseSpeed)
		}
		if (heldMovementKeys[mouseLeft]) {
			mouse.Move(-mouseSpeed, 0)
		}
		if (heldMovementKeys[mouseDown]) {
			mouse.Move(0, mouseSpeed)
		}
		if (heldMovementKeys[mouseRight]) {
			mouse.Move(mouseSpeed, 0)
		}
		if (heldMovementKeys[scrollUp]) {
			mouse.Wheel(false, scrollSpeed)
		}
		if (heldMovementKeys[scrollLeft]) {
			mouse.Wheel(true, -scrollSpeed)
		}
		if (heldMovementKeys[scrollDown]) {
			mouse.Wheel(false, -scrollSpeed)
		}
		if (heldMovementKeys[scrollRight]) {
			mouse.Wheel(true, scrollSpeed)
		}
		movementMutex.RUnlock()
		time.Sleep(16 * time.Millisecond)
	}
}

// Kill the overlay process
func terminateOverLay() {
	if overlayProc != nil && overlayProc.Process != nil {
		_ = overlayProc.Process.Kill()
	}
	exec.Command("pkill", "-f", "overlay.py").Run()
}

// Display the overlay showing the division
func showOverlay() {
	messageOverlay([]byte(fmt.Appendf([]byte{},"show, %v, %v, %v", divCount, currentDivBoxX, currentDivBoxY)))
}

// Hide the overlay
func hideOverlay() {
	messageOverlay([]byte("hide"))
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

// Move mouse to the center of the selected box during a division
func mouseToBox() {

	// Find index of pressed key in key map
	var selectedKeyId int32 = divKeyMaps[divCount][pressed]	

	// Calculate which box is selected
	currentDivBoxX = currentDivBoxX + float32(selectedKeyId % divDim[divCount][0]) * divArea[divCount+1][0]
	currentDivBoxY = currentDivBoxY + float32(selectedKeyId / divDim[divCount][0]) * divArea[divCount+1][1]

	// Move mouse to the center of the box
	var moveMouseX = int32(currentDivBoxX + divArea[divCount+1][0] / 2)
	var moveMouseY = int32(currentDivBoxY + divArea[divCount+1][1] / 2)
	mouseAbs(moveMouseX, moveMouseY)
}

// Reset mousemode and enter div mode
func enterDivMode() {
	mouseMode = false
	movementMutex.Lock()
	for key, _ := range heldMovementKeys {
		heldMovementKeys[key] = false
	}
	movementMutex.Unlock()
	divMode = true // Must clear heldMovementKeys when entering divMode
	divCount ++
	showOverlay()
}

// Reset division related variables
func exitDivMode() {
	hideOverlay()
	divMode = false
	divCount = -1
	currentDivBoxX = 0
	currentDivBoxY = 0
	pressed = ""
	if (clickAfterSelect) {
		time.Sleep(10 * time.Millisecond)
		mouse.LeftClick()
	}
	if (mouseModeAfterSelect) {
		mouseMode = true
	}
}

// Main loop
func main() {

	// Load configs
	load_config(defaultConfigPath, true)
	load_config(userConfigPath, false)
	set_config()
	verify_config()
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

	go movementLoop()

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
				if (!mouseMode && !divMode) {
					// If div mode can be activated without mouse mode, enter div mode
					if (!divAfterMouse && keyNames[event.Code] == divKey && event.Value == 1) {
						enterDivMode()
						continue
					}
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

				// Mouse mode behaviors, register holding and releasing movement keys
				if (mouseMode) {
					var code string = keyNames[event.Code]
					if _, ok := heldMovementKeys[code]; ok {
						movementMutex.Lock()
						switch (event.Value) {
							case 1: 
								heldMovementKeys[code] = true
							case 0:
								heldMovementKeys[code] = false
						}
						movementMutex.Unlock()
					}

					if (event.Value == 1 && keyNames[event.Code] == mouseClick) {
						mouse.LeftPress()
					} else if (event.Value == 0 && keyNames[event.Code] == mouseClick) {
						mouse.LeftRelease()
					} else if (event.Value == 1 && keyNames[event.Code] == mouseRightClick) {
						mouse.RightPress()
					} else if (event.Value == 0 && keyNames[event.Code] == mouseRightClick) {
						mouse.RightRelease()
					}
				}

				// Mousemode and user has pressed key to enter overlay
				if (mouseMode && keyNames[event.Code] == divKey && event.Value == 1) {
					enterDivMode()
					continue
				}

				// Div mode behaviors
				if (divMode && event.Value == 1) {
					pressed += keyNames[event.Code]
					if _,ok := divKeyMaps[divCount][pressed]; ok {
						mouseToBox()
						if (divCount == nDivs - 1) {
							divMode = false
							exitDivMode()
						} else if (divCount < nDivs - 1) {
							pressed = ""
							divCount ++
							showOverlay()
						}
					} else {
						if (len(pressed) >= longestKeyLen[divCount]) {
							divMode = false
							exitDivMode()
						}
					}
				}
			}
		}
	}
}
