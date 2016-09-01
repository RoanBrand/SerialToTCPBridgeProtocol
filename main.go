package main

import (
	"github.com/RoanBrand/SerialToTCPBridgeProtocol/protocol"
	"log"
)

func main() {
	done := make(chan bool)
	com, err := protocol.NewComHandler("COM3", 115200, done)
	if err != nil {
		log.Fatalf("Error starting COM Handler: %v", err)
	}
	defer com.EndGracefully()

	log.Println("Starting Server")
	_ = <-done
}
