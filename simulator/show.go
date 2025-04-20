package main

import (
	"fmt"
	"log"

	"go.bug.st/serial/enumerator"
)

func ShowUSBPorts() {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		fmt.Println("Error enumerating ports:", err)
		log.Fatal(err)
		return
	}

	if len(ports) == 0 {
		fmt.Println("No serial ports found!")
		return
	}

	fmt.Printf("Found USB ports: %d\n", len(ports))
	for _, port := range ports {
		fmt.Printf("Product: %s\n", port.Product)
		fmt.Printf("Port name: %s\n", port.Name)
	}

	fmt.Printf("Found USB ports\n")
	// print all usb ports
	for _, port := range ports {
		if port.IsUSB {
			fmt.Printf("Product: %s\n", port.Product)
			fmt.Printf("Port name: %s\n", port.Name)
		}
	}
}
