package main

import (
	"github.com/RoanBrand/SerialToTCPBridgeProtocol/protocol"
	"log"
	"net"
)

func main() {

	tcpCon, err := net.Dial("tcp", "127.0.0.1:1883")
	if err != nil {
		log.Fatal("Error opening TCP Port")
	}
	defer tcpCon.Close()

	com, err := protocol.NewComHandler(&tcpCon)
	if err != nil {
		log.Fatal("Error opening COM Port")
	}
	defer com.EndGracefully()

	log.Println("Starting Server")
	for {}

}
