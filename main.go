package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	helper "github.com/CB2Moon/DJI_RC_Nx_Translator/pkg"
	"github.com/CB2Moon/vgamepad-go/pkg/commons"
	"github.com/CB2Moon/vgamepad-go/pkg/vgamepad"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

// Global variables
var (
	sequenceNumber uint16 = 0x34eb
	stickPositions        = map[string]int16{"right_horizontal": 0, "right_vertical": 0, "left_horizontal": 0, "left_vertical": 0}
	cameraDial     int16  = 0
	serialPort     serial.Port
	stopChan       = make(chan bool)
	gamepad        *vgamepad.VX360Gamepad

	// UI related globals
	mainWindow  *walk.MainWindow
	logView     *walk.TextEdit
	statusLabel *walk.Label
	startButton *walk.PushButton
	stopButton  *walk.PushButton
	exitButton  *walk.PushButton
)

// Logger function that redirects log output to UI
func uiLogger(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)

	// Update UI safely from any goroutine
	if mainWindow != nil {
		mainWindow.Synchronize(func() {
			if logView != nil {
				logView.AppendText(msg + "\r\n")
				// Auto-scroll to bottom
				logView.SendMessage(0x115, 7, 0) // WM_VSCROLL, SB_BOTTOM
			}
		})
	}

	fmt.Println(msg)
}

// Update status label safely
func updateStatus(status string) {
	if mainWindow != nil {
		mainWindow.Synchronize(func() {
			if statusLabel != nil {
				statusLabel.SetText(status)
			}
		})
	}
}

// sendDUML sends a DUML (DJI Universal Markup Language) protocol packet over serial port.
func sendDUML(port serial.Port, sourceAddress, targetAddress, commandType, commandSet, commandID byte, payload []byte) error {
	if port == nil {
		return fmt.Errorf("serial port is nil")
	}

	packet, err := helper.BuildDUML(sequenceNumber, sourceAddress, targetAddress, commandType, commandSet, commandID, payload)
	if err != nil {
		return err
	}

	if _, err = port.Write(packet); err != nil {
		return err
	}

	sequenceNumber++
	return nil
}

// parseInput converts RC-Nx (364 to 1024 to 1684) stick input values to Xbox controller range (-32768 to 0 to 32767)
func parseInput(rawInput []byte) int16 {
	value := int(binary.LittleEndian.Uint16(rawInput)) - 1024
	mappedValue := value * 2 * 4096 / 165
	if mappedValue >= 32768 {
		mappedValue = 32767
	}
	return int16(mappedValue)
}

// getStickStatus returns the stick status if the packet is valid
func getStickStatus(packet []byte) (rightHorizontal, rightVertical, leftVertical, leftHorizontal, cameraDial int16, err error) {
	if err := helper.ValidatePacket(packet); err != nil {
		return 0, 0, 0, 0, 0, err
	}

	return parseInput(packet[13:15]), parseInput(packet[16:18]), parseInput(packet[19:21]), parseInput(packet[22:24]), parseInput(packet[25:27]), nil
}

// translateN1MovementAndUpdateGamepad continuously updates virtual gamepad state
func translateN1MovementAndUpdateGamepad() {
	uiLogger("Gamepad update loop started.")
	for {
		select {
		case <-stopChan:
			uiLogger("Gamepad update loop stopped.")
			return
		default:
			time.Sleep(100 * time.Millisecond)

			if gamepad == nil {
				continue
			}

			gamepad.LeftJoystick(stickPositions["left_horizontal"], stickPositions["left_vertical"])
			gamepad.RightJoystick(stickPositions["right_horizontal"], stickPositions["right_vertical"])

			// leftH := stickPositions["left_horizontal"]
			// leftV := stickPositions["left_vertical"]
			// rightH := stickPositions["right_horizontal"]
			// rightV := stickPositions["right_vertical"]

			// log.Printf("Left: (%d, %d), Right: (%d, %d), Camera: %d\n",
			// 	leftH, leftV, rightH, rightV, cameraDial)

			if cameraDial > 32000 {
				// log.Println("Pressing Y button (restart race)")
				gamepad.PressButton(commons.XUSB_GAMEPAD_Y)
			} else if cameraDial < -32000 {
				// log.Println("Pressing B button (recover drone)")
				gamepad.PressButton(commons.XUSB_GAMEPAD_B)
			} else {
				// log.Println("Releasing buttons")
				gamepad.ReleaseButton(commons.XUSB_GAMEPAD_Y)
				gamepad.ReleaseButton(commons.XUSB_GAMEPAD_B)
			}

			if err := gamepad.Update(); err != nil {
				uiLogger("Error updating gamepad state: %v", err)
			}
		}
	}
}

// startControllerProcess starts the main controller processing
func startControllerProcess() error {
	// a test gamepad to ensure ViGEmBus is installed
	updateStatus("Initializing - Checking driver...")
	uiLogger("Checking ViGEmBus driver installation...")
	testGamepad, err := vgamepad.NewVX360Gamepad()
	if err != nil {
		updateStatus("Error - Driver issue")
		return fmt.Errorf("failed to initialize virtual gamepad (ViGEmBus driver issue): %v", err)
	}
	testGamepad.Close()

	updateStatus("Scanning for DJI controller...")
	uiLogger("Scanning for DJI USB ports...")
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		updateStatus("Error scanning ports")
		return fmt.Errorf("could not get port list: %v", err)
	}

	portName := ""
	foundPort := false
	for _, port := range ports {
		// For testing, use Electronic Team Virtual Serial Port (should create for global session)
		// if strings.Contains(strings.ToLower(port.Product), strings.ToLower("Electronic Team Virtual Serial Port")) {
		if port.IsUSB && port.Product == "DJI USB VCOM For Protocol" {
			uiLogger("Found DJI USB VCOM For Protocol on %s", port.Name)
			portName = port.Name
			foundPort = true
			break
		}
	}

	if !foundPort {
		// Attempting a default or showing error might be better than assuming COM9
		// portName = "COM9"
		// uiLogger("DJI controller not detected. Using default port: %s", portName)
		updateStatus("DJI controller not detected. Check connection.")
		return fmt.Errorf("DJI USB VCOM For Protocol not found")
	}

	uiLogger("Opening serial port: %s", portName)
	mode := &serial.Mode{
		BaudRate: 115200,
	}
	port, err := serial.Open(portName, mode)
	if err != nil {
		updateStatus("Failed to open serial port")
		return fmt.Errorf("could not open serial port: %v", err)
	}
	serialPort = port

	uiLogger("Creating Virtual X360 Gamepad...")
	gp, err := vgamepad.NewVX360Gamepad()
	if err != nil {
		serialPort.Close()
		serialPort = nil
		updateStatus("Failed to create virtual gamepad")
		uiLogger("Error creating gamepad: %v", err)
		return fmt.Errorf("could not create gamepad: %w", err)
	}
	gamepad = gp
	gamepad.Reset()
	uiLogger("Virtual gamepad created successfully.")

	uiLogger("Starting translator process...")
	updateStatus("Running")
	safeGoroutine("GamepadUpdate", translateN1MovementAndUpdateGamepad)
	safeGoroutine("SerialReadLoop", serialReadLoop)

	return nil
}

// serialReadLoop's original version
func serialReadLoop() {
	// Enable simulator mode for RC to get faster stick position updates
	if err := sendDUML(serialPort, 0x0a, 0x06, 0x40, 0x06, 0x24, []byte{0x01}); err != nil {
		uiLogger("Error sending DUML command: %v", err)
	}

	buffer := make([]byte, 1024)

	for {
		select {
		case <-stopChan:
			uiLogger("Serial read loop stopped")
			return
		default:
			if serialPort == nil {
				uiLogger("Serial port is no longer available")
				return
			}
			// Request latest channel values from RC
			if err := sendDUML(serialPort, 0x0a, 0x06, 0x40, 0x06, 0x01, nil); err != nil {
				uiLogger("Error requesting channel values: %v", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Read packet header
			packetBuffer, packetLength, err := helper.ReadPacketHeader(serialPort, buffer)
			if err != nil {
				uiLogger("Error reading packet header: %v", err)
				continue
			}

			// Read packet body
			remainingBytes := int(packetLength) - 4
			if remainingBytes > 0 {
				n, err := helper.ReadBytes(serialPort, buffer, remainingBytes)
				if err != nil || n != remainingBytes {
					uiLogger("Error reading packet body: %v (expected: %d, actual: %d)", err, remainingBytes, n)
					continue
				}
				packetBuffer = append(packetBuffer, buffer[:n]...)
			}

			// Parse stick positions from 38-byte controller input packets
			if len(packetBuffer) == 38 {
				// Map raw values to virtual controller ranges
				rightHorizontal, rightVertical, leftVertical, leftHorizontal, cd, err := getStickStatus(packetBuffer)
				if err != nil {
					uiLogger("Error validating packet: %v", err)
					continue
				}
				// Update stick positions
				stickPositions["right_horizontal"] = rightHorizontal
				stickPositions["right_vertical"] = rightVertical
				stickPositions["left_vertical"] = leftVertical
				stickPositions["left_horizontal"] = leftHorizontal
				cameraDial = cd
			}

			time.Sleep(10 * time.Millisecond)
		}
	}
}

// cleanupAndExit performs cleanup before exiting
func cleanupAndExit() {
	uiLogger("Shutting down...")
	select {
	case <-stopChan:
		// Already closed
	default:
		close(stopChan)
	}

	// Wait briefly for goroutines to finish (optional, needs sync.WaitGroup for reliability)
	time.Sleep(200 * time.Millisecond)

	// Close resources
	if serialPort != nil {
		uiLogger("Closing serial port...")
		if err := serialPort.Close(); err != nil {
			uiLogger("Error closing serial port: %v", err)
		}
		serialPort = nil
	}

	if gamepad != nil {
		uiLogger("Cleaning up gamepad...")
		gamepad.Close()
		gamepad = nil
	}

	uiLogger("Shutdown complete.")
	mainWindow.Synchronize(func() {
		walk.App().Exit(0)
	})
}

func safeGoroutine(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				stack := make([]byte, 4096)
				length := runtime.Stack(stack, false)
				uiLogger("PANIC in %s goroutine: %v\n%s", name, r, stack[:length])

				// Also log to file for debugging
				f, err := os.OpenFile("panic_log.txt", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
				if err == nil {
					defer f.Close()
					fmt.Fprintf(f, "PANIC in %s goroutine: %v\n%s\n", name, r, stack[:length])
				}
			}
		}()

		fn()
	}()
}

func main() {
	// Create a log file to capture startup errors
	logFile, err := os.OpenFile("startup_log.txt", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		defer logFile.Close()
		log.SetOutput(logFile)
	}

	err = runApplication()
	if err != nil {
		log.Printf("Fatal error: %v", err)
		// If possible show error in message box before exiting
		if mainWindow != nil {
			walk.MsgBox(nil, "Startup Error",
				fmt.Sprintf("Application failed to start: %v", err),
				walk.MsgBoxIconError)
		}
		os.Exit(1)
	}
}

func runApplication() error {
	// Create and display the UI window
	err := createMainWindow()
	if err != nil {
		return fmt.Errorf("failed to create main window: %w", err)
	}

	// Set initial status
	updateStatus("Ready - Click Start")
	uiLogger("Application initialized. Waiting for user action.")

	// Main message loop
	mainWindow.Run()

	uiLogger("Application exiting.")
	return nil
}

func loadAppIcon() (*walk.Icon, error) {
	icon, err := walk.NewIconFromResourceId(2) // 2 is the standard ID for main application icon
	if err == nil {
		return icon, nil
	}

	// Fallback to loading from the embedded file
	iconPath, err := getIconPath()
	if err != nil {
		return nil, err
	}

	return walk.NewIconFromFile(iconPath)
}

// Wrap main window creation in a function with error handling
func createMainWindow() error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic in createMainWindow: %v", r)
			f, err := os.OpenFile("startup_log.txt", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
			if err == nil {
				defer f.Close()
				fmt.Fprintf(f, "PANIC: %v\n", r)
			}
		}
	}()

	appIcon, err := loadAppIcon()
	if err != nil {
		uiLogger("Failed to load application icon: %v", err)
	}

	return MainWindow{
		AssignTo: &mainWindow,
		Title:    "DJI RC-Nx to Xbox Controller Translator",
		MinSize:  Size{Width: 400, Height: 200},
		Layout:   VBox{},
		Icon:     appIcon,
		OnBoundsChanged: func() { // Auto-scroll log view on resize/init
			if logView != nil {
				logView.SendMessage(0x115, 7, 0)
			}
		},
		Children: []Widget{
			VSplitter{
				Children: []Widget{
					Label{
						AssignTo: &statusLabel,
						Text:     "Status: Initializing...",
					},
					TextEdit{
						AssignTo: &logView,
						ReadOnly: true,
						VScroll:  true,
						Font:     Font{Family: "Consolas", PointSize: 14},
						Text:     "Welcome to DJI RC-Nx to Xbox Controller Translator\r\n",
					},
				},
			},
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					PushButton{
						AssignTo: &startButton,
						Text:     "Start",
						MaxSize:  Size{Width: 2000, Height: 0},
						OnClicked: func() {
							startButton.SetEnabled(false)
							stopButton.SetEnabled(true)

							err := startControllerProcess()
							if err != nil {
								uiLogger("Error: %v", err)
								walk.MsgBox(mainWindow, "Error",
									fmt.Sprintf("Failed to start: %v", err),
									walk.MsgBoxIconError)
								startButton.SetEnabled(true)
								stopButton.SetEnabled(false)
							}
						},
					},
					PushButton{
						AssignTo: &stopButton,
						Text:     "Stop",
						MaxSize:  Size{Width: 2000, Height: 0},
						Enabled:  false,
						OnClicked: func() {
							stopButton.SetEnabled(false)
							startButton.SetEnabled(true)

							close(stopChan)
							stopChan = make(chan bool) // Recreate channel for next start

							// Wait for goroutines to finish
							time.Sleep(300 * time.Millisecond)

							if serialPort != nil {
								serialPort.Close()
								serialPort = nil
							}

							if gamepad != nil {
								gamepad.Close()
								gamepad = nil
							}

							updateStatus("Stopped")
							uiLogger("Translator stopped.")
						},
					},
					PushButton{
						AssignTo: &exitButton,
						Text:     "Exit",
						MaxSize:  Size{Width: 2000, Height: 0},
						OnClicked: func() {
							cleanupAndExit()
							mainWindow.Close()
						},
					},
				},
			},
		},
	}.Create()
}
