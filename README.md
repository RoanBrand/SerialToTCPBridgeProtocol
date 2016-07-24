# SerialToTCPBridgeProtocol
 An error tolerant serial UART to TCP connection, raw data bridge.

PC side service written in Go that listens on COM port. STM32F334 Nucleo board client written in C.

Currently, there is a MQTT protocol implementation over this connection as an example.
The microcontroller is effectively making a connection to a MQTT broker over the virtual COM USB.

#### Details
- The Go service opens a real TCP connection to a set destination on behalf of the STM32 Serial Client.
- The Protocol utilizes the internal CRC32 unit on the STM32 for communication error checking.
- The Project for the STM32 chip is assembled using the Atollic TrueSTUDIO for ARM IDE with the STM32CubeMX plugin.
- FreeRTOS is used on the STM32 and a stripped down version of [knolleary's MQTT library for Arduino](https://github.com/knolleary/pubsubclient).

#### Future plans
- Sort out timeout bugs
- Unit tests
- Extend protocol that client can open "tcp" connection by specifying IP destination & port
- PC side service to listen to all COM ports and concurrently spawn new connections for clients
- Create a Arduino lib that extends the [Arduino Client class](https://www.arduino.cc/en/Reference/ClientConstructor) so that libraries for existing Ethernet/Wifi shields can theoretically work.
