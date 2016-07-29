package main

import (
	"github.com/RoanBrand/SerialToTCPBridgeProtocol/protocol"
	"log"
)

func main() {
	com, err := protocol.NewComHandler("COM6", 115200)
	if err != nil {
		log.Fatalf("Error starting COM Handler: %v", err)
	}
	defer com.EndGracefully()

	log.Println("Starting Server")
	for {}
}
