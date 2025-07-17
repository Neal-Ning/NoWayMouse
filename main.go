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
	// the fact that overlay.py does not cover the way bar
	mouse.Move(x, y * (resY - waybarHeight) / resY + waybarHeight)
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
	messageOverlay([]byte(fmt.Sprintf("show, %v, %v, %v, %v, %v, %v, %v",
		divCount, currentDivBoxX, currentDivBoxY,
		divArea[divCount][0], divArea[divCount][1],
		divDim[divCount][0], divDim[divCount][1])))
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
	currentDivBoxX = currentDivBoxX + selectedKeyId % divDim[divCount][0] * divArea[divCount+1][0]
	currentDivBoxY = currentDivBoxY + selectedKeyId / divDim[divCount][0] * divArea[divCount+1][1]

	// Move mouse to the center of the box
	var moveMouseX int32 = currentDivBoxX + divArea[divCount+1][0] / 2
	var moveMouseY int32 = currentDivBoxY + divArea[divCount+1][1] / 2
	mouseAbs(moveMouseX, moveMouseY)
}

// Reset division related variables
func exitDivMode() {
	hideOverlay()
	divMode = false
	divCount = -1
	currentDivBoxX = 0
	currentDivBoxY = 0
	pressed = ""
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

				// Register holding and releasing movement keys
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
						mouse.LeftClick()
					}
				}

				// Mousemode and user has pressed key to enter overlay
				if (mouseMode && keyNames[event.Code] == divKey && event.Value == 0) {
					mouseMode = false
					divMode = true // Must clear heldMovementKeys when entering divMode
					divCount ++
					showOverlay()
					continue
				}

				if (divMode && event.Value == 0) {
					pressed += keyNames[event.Code]
					if _,ok := divKeyMaps[divCount][pressed]; ok {
						fmt.Println("Pressed in map")
						mouseToBox()
						if (divCount == nDivs - 1) {
							divMode = false
							exitDivMode()
						} else if (divCount < nDivs - 1) {
							pressed = ""
							showOverlay()
							divCount ++
						}
					} else {
						fmt.Println("Pressed not in map: ", pressed)
						if (len(pressed) >= longestKeyLen[divCount]) {
							divMode = false
							exitDivMode()
						}
					}
				}

				// // User has pressed key in overlay mode
				// if (divMode && event.Value == 0) {
				// 	// If current keystroke is the first input after overlay mode is activated
				// 	if (!overlay01) {
				// 		selectedDivCol = div0XKeyMap[keyNames[event.Code]]
				// 		overlay01 = true

				// 	// If current keystroke is the second input after overlay mode is activated
				// 	} else if (overlay01 && !overlay02) {

				// 		// Calculate coordinates
				// 		if (selectedDivCol < div0Cols / 2) {
				// 			selectedDivRow = div0Y0KeyMap[keyNames[event.Code]]
				// 		} else {
				// 			selectedDivRow = div0Y1KeyMap[keyNames[event.Code]]
				// 		}
				// 		overlay02 = true

				// 		// Calculate the absolute position of the center of the selected box
				// 		var selectedBox0Xabs = selectedDivCol * box0X + box0X / 2
				// 		var selectedBox0Yabs = selectedDivRow * box0Y + box0Y / 2

				// 		// Move the mouse to the center of the selected box and display second overlay
				// 		mouseAbs(selectedBox0Xabs, selectedBox0Yabs)
				// 		showOverlay1(selectedDivCol, selectedDivRow)

				// 		if (adjustMode == 1) {
				// 			exitDivMode()
				// 			mouseMode = true
				// 		} else if (adjustMode == 2) {
				// 			continue
				// 		}

				// 	// If current keystroke is the third input after overlay mode is activated
				// 	} else if (overlay01 && overlay02 && !overlay1) {
				// 		var selectedBox1 int32 = div1KeyMap[keyNames[event.Code]]

				// 		// Calculate the absolute position of the center of the selected box
				// 		var selectedBox1Xabs = selectedDivCol * box0X + selectedBox1 % div1Cols * box1X + box1X / 2
				// 		var selectedBox1Yabs = selectedDivRow * box0Y + selectedBox1 / div1Cols * box1Y + box1Y / 2

				// 		// Move the mouse to the center of the selected box and exit overlay
				// 		mouseAbs(selectedBox1Xabs, selectedBox1Yabs)
				// 		exitDivMode()

				// 		// Wait for overlay to exit
				// 		time.Sleep(10 * time.Millisecond)
				// 		mouse.LeftClick()
				// 	}
				// }
			}
		}
	}
}
