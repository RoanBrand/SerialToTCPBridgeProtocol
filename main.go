package main

import (
	"github.com/RoanBrand/SerialToTCPBridgeProtocol/protocol"
	"log"
	"time"
)

const (
	COMPortName = "COM3"
	COMBaudRate = 115200
)

func main() {
	com := protocol.NewComHandler(COMPortName, COMBaudRate)
	for {
		err := com.ListenAndServeClient()
		log.Printf("%v: Error: %v\n", COMPortName, err)
		log.Println("Retrying in 5s...")
		<-time.After(time.Second * 5)
	}
}
