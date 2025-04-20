package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	helper "github.com/CB2Moon/DJI_RC_Nx_Translator/pkg"
	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

// Global variables
var (
	realPort       string
	simulatedPort  string
	sequenceNumber uint16 = 0x4321
	isRunning      bool   = false
	verbose        bool
	port           serial.Port
)

// sendDUML sends a DUML protocol packet to the host software
func sendDUML(port serial.Port, sourceAddress, targetAddress, commandType, commandSet, commandID byte, payload []byte) error {
	if port == nil {
		return fmt.Errorf("serial port is nil")
	}

	packet, err := helper.BuildDUML(sequenceNumber, sourceAddress, targetAddress, commandType, commandSet, commandID, payload)
	if err != nil {
		return err
	}

	// Send packet
	if _, err = port.Write(packet); err != nil {
		return err
	}

	sequenceNumber++
	return nil
}

// parseDUMLPacket parses a DUML packet and returns its components
func parseDUMLPacket(packet []byte) (sourceAddr, targetAddr, cmdType, cmdSet, cmdID byte, payload []byte, err error) {
	if len(packet) < 13 {
		return 0, 0, 0, 0, 0, nil, fmt.Errorf("packet too short")
	}

	if packet[0] != 0x55 {
		return 0, 0, 0, 0, 0, nil, fmt.Errorf("invalid packet header: expected 0x55, got 0x%02X", packet[0])
	}

	// Extract packet length
	packetHeader := binary.LittleEndian.Uint16(packet[1:3])
	packetLength := packetHeader & 0x03FF

	if int(packetLength) != len(packet) {
		return 0, 0, 0, 0, 0, nil, fmt.Errorf("invalid packet length: expected %d, got %d", len(packet), int(packetLength))
	}

	// Verify header checksum
	hdrCrc := helper.CalcPkt55HdrChecksum(0x77, packet[:3], 3)
	if hdrCrc != packet[3] {
		return 0, 0, 0, 0, 0, nil, fmt.Errorf("header checksum mismatch: expected 0x%02X, got 0x%02X", hdrCrc, packet[3])
	}

	// Verify packet checksum
	crcExpected := binary.LittleEndian.Uint16(packet[len(packet)-2:])
	crcCalculated := helper.CalcChecksum(packet[:len(packet)-2], len(packet)-2)
	if crcExpected != crcCalculated {
		return 0, 0, 0, 0, 0, nil, fmt.Errorf("packet checksum mismatch: expected 0x%04X, got 0x%04X", crcExpected, crcCalculated)
	}

	// Extract components
	sourceAddr = packet[4]
	targetAddr = packet[5]
	// Sequence number is at 6-7
	cmdType = packet[8]
	cmdSet = packet[9]
	cmdID = packet[10]

	if len(packet) > 13 {
		payload = packet[11 : len(packet)-2]
	} else {
		payload = nil
	}

	return sourceAddr, targetAddr, cmdType, cmdSet, cmdID, payload, nil
}

// createStickDataPacket creates a simulated RC stick data packet
func createStickDataPacket(rightH, rightV, leftV, leftH, camera int16) []byte {
	// Convert joystick values from Xbox range (-32768 to 32767) to RC Nx range (364 to 1024 to 1684)
	convValue := func(value int16) uint16 {
		// Apply inverse of parseInput function from the original program
		// Original: mappedValue := (value-1024) * 2 * 4096 / 165
		// Inverse:  value := mappedValue * 165 / (2 * 4096) + 1024
		if value > 32767 {
			value = 32767
		}
		if value < -32768 {
			value = -32768
		}

		rawValue := (int(value) * 165) / (2 * 4096)
		return uint16(rawValue + 1024)
	}

	// Convert Xbox joystick values to RC Nx values
	rhVal := convValue(rightH)
	rvVal := convValue(rightV)
	lvVal := convValue(leftV)
	lhVal := convValue(leftH)
	cVal := convValue(camera)

	// Create stick data packet payload to make up a 38-byte packet
	payload := make([]byte, 25)

	// Insert stick positions at the expected offsets (based on the main program's parsing logic)
	// Offsets are adjusted since sendDUML will add the header
	binary.LittleEndian.PutUint16(payload[2:4], rhVal)   // right_horizontal at offset 13-15 in full packet
	binary.LittleEndian.PutUint16(payload[5:7], rvVal)   // right_vertical at offset 16-18 in full packet
	binary.LittleEndian.PutUint16(payload[8:10], lvVal)  // left_vertical at offset 19-21 in full packet
	binary.LittleEndian.PutUint16(payload[11:13], lhVal) // left_horizontal at offset 22-24 in full packet
	binary.LittleEndian.PutUint16(payload[14:16], cVal)  // camera dial at offset 25-27 in full packet

	return payload
}

// generateMotion simulates stick movement patterns
func generateMotion(t float64) (rightH, rightV, leftV, leftH, camera int16) {
	// Generate circular motion for right stick
	rightH = int16(25000 * math.Sin(t))
	rightV = int16(25000 * math.Cos(t))

	// Generate figure-eight pattern for left stick
	leftH = int16(20000 * math.Sin(t))
	leftV = int16(20000 * math.Sin(t*2))

	// Generate oscillating camera dial
	camera = int16(30000 * math.Sin(t/2))

	return
}

// go run main.go -port COM2 -verbose
func main() {
	// Parse command line flags, e.g. -port COM5 -verbose
	comPort := flag.String("port", "", "COM port to use (if not specified, will use the first available port)")
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose logging")
	flag.Parse()

	log.Println("DJI RC-Nx Simulator starting...")
	log.Println("This program simulates a DJI remote controller for testing purposes")

	// Find available ports
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		log.Fatalf("Error getting port list: %v", err)
	}

	// If no port specified, use the first available
	portName := *comPort
	if portName == "" {
		if len(ports) == 0 {
			log.Fatalf("No COM ports available")
		}
		for _, p := range ports {
			// Skip ports that might already be in use
			if p.IsUSB && !strings.Contains(p.Name, "bluetooth") {
				portName = p.Name
				break
			}
		}
		if portName == "" {
			log.Fatalf("No suitable COM port found")
		}
	}

	log.Printf("Using COM port: %s", portName)

	// Open the selected port
	mode := &serial.Mode{
		BaudRate: 115200,
	}

	port, err = serial.Open(portName, mode)
	if err != nil {
		log.Fatalf("Failed to open COM port: %v", err)
	}
	defer port.Close()

	log.Printf("Port opened successfully. Simulating DJI USB VCOM For Protocol")
	log.Printf("Waiting for commands from the translator program...")

	// Set up signal handling for clean shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the main processing loop in a goroutine
	go processLoop()

	// Wait for signal
	<-sigChan
	log.Println("Shutting down...")
}

// handleStickDataRequest processes stick data requests
func handleStickDataRequest(port serial.Port) error {
	t := float64(time.Now().UnixNano()) / 1e9
	rightH, rightV, leftV, leftH, camera := generateMotion(t)
	stickData := createStickDataPacket(rightH, rightV, leftV, leftH, camera)
	return sendDUML(port, 0x06, 0x0a, 0x40, 0x06, 0x01, stickData)
}

func processLoop() {
	buffer := make([]byte, 1024)

	for {
		// Read packet header
		packetBuffer, packetLength, err := helper.ReadPacketHeader(port, buffer)
		if err != nil {
			if verbose {
				log.Printf("Header read error: %v", err)
			}
			continue
		}

		// Read packet body
		remainingBytes := int(packetLength) - 4
		if remainingBytes > 0 {
			n, err := helper.ReadBytes(port, buffer, remainingBytes)
			if err != nil || n != remainingBytes {
				if verbose {
					log.Printf("Body read error: %v", err)
				}
				continue
			}
			packetBuffer = append(packetBuffer, buffer[:n]...)
		}

		_, _, cmdType, cmdSet, cmdID, payload, err := parseDUMLPacket(packetBuffer)
		if err != nil {
			log.Printf("Error parsing packet: %v", err)
			continue
		}

		// Process commands
		if cmdType == 0x40 && cmdSet == 0x06 {
			switch cmdID {
			case 0x01:
				if verbose {
					log.Println("Received channel values request, sending stick data")
				}
				if err := handleStickDataRequest(port); err != nil {
					log.Printf("Error sending stick data: %v", err)
				}
			case 0x24:
				if len(payload) > 0 && payload[0] == 0x01 {
					log.Println("Simulator mode enabled")
					isRunning = true
				}
			}
		}
	}
}
