# Serial to TCP Bridge Protocol
 An error tolerant serial UART to TCP connection, raw data bridge.

[See Documentation](https://roanbrand.github.io/SerialToTCPBridgeProtocol/)  

#### Description
TCP Connections over Serial.  
Useful to connect microcontrollers on development boards like the Arduino Uno to servers, just through the Serial port over USB, without requiring any Ethernet/Wi-Fi hardware.  
Meant to bridge the gap between TCP connections to servers and simple serial devices using UART/RS-232/Serial-over-USB, etc.  


Host side gateway service written in Go that listens on Serial ports for clients.  
Clients implementing the protocol client have a TCP-like API that they can use to make connections to real servers, without networking hardware.  
The goal of the project is to have the means to connect the simplest and cheapest devices to the internet, albeit indirectly.  


Included in this repo is an implementation of the Protocol **Gateway** and **Client**, written in Go. They work on Windows, Linux, Raspberry Pi OS.  
The following clients are also available:

| Client                                                                                         | Platform | Language |
| ---------------------------------------------------------------------------------------------- |:--------:|:--------:|
| [ArduinoSerialToTCPBridgeClient](https://github.com/RoanBrand/ArduinoSerialToTCPBridgeClient)  | Arduino  | C++      |
| [STM32SerialToTCPBridgeClient](https://github.com/RoanBrand/STM32SerialToTCPBridgeClient)      | STM32    | C        |

#### Build and Run
- Install [Go](https://go.dev/dl/) for your system.
- Run `go install github.com/RoanBrand/SerialToTCPBridgeProtocol@latest` in a terminal.
- Copy *config.json* from the repository and the installed executable `~/go/bin/SerialToTCPBridgeProtocol` to a new folder.
- Edit your local `config.json` and set it according to your Serial port configuration.
- Run the `SerialToTCPBridgeProtocol` executable.

#### Details
- The protocol provides the app an in order, duplicates free and error checked byte stream by adding a CRC32 and simple retry mechanism. See [this](https://en.wikibooks.org/wiki/Serial_Programming/Error_Correction_Methods) for background.
- The **Protocol Gateway** opens a real TCP connection to a set destination on behalf of the Protocol Client.
- The **Protocol Client** connects to the Protocol Gateway over a serial-like connection, which can possibly corrupt data.
- The client specifies the destination IPv4 address and port.
- The gateway forwards traffic bi-directionally, as long as tcp connection is open and serial line is good.

#### Tests
 - Open a terminal, then run `go get -u github.com/RoanBrand/goBuffers`.
 - In the terminal, change directory to the `protocol` folder inside the repository.
 - Run `go test -v` in the terminal.

#### Future plans
- Add ping option to periodically test serial line and drop upstream connection if timeout.
- Multiple connections per client to servers.
- Capability to scan system and listen on all found COM ports for clients.
- Turn into OS service.
