/*
 * tcpAbstraction.h
 *
 *  Created on: Jul 9, 2016
 *      Author: Roan
 */

#ifndef TCPABSTRACTION_H_
#define TCPABSTRACTION_H_

#include <stdbool.h>
#include "stm32f3xx_hal.h"

#define SERIAL_TIMEOUT 400

#define PROTOCOL_CONNECT 0
#define PROTOCOL_CONNACK 1
#define PROTOCOL_DISCONNECT 2
#define PROTOCOL_PUBLISH 3
#define PROTOCOL_ACK 4

#define STATE_DISCONNECTED 0
#define STATE_CONNECTED 1

typedef struct Client_t
{
	UART_HandleTypeDef* peripheral_UART;
	CRC_HandleTypeDef* peripheral_CRC;

	uint8_t rxByte;
	uint8_t rxBuffer[256];
	uint8_t pRx_rx, pRead_rx;
	bool rxFull;

	uint8_t readBuffer[256];
	uint8_t pRx_read, pRead_read;
	bool readFull;

	uint8_t workBuffer[128];
	bool txReady;
	bool ackOutstanding;
	bool sequenceTxFlag;
	bool expectedRxSeqFlag;
	uint32_t lastInAct, lastOutAct;
	uint8_t state;

	bool (*start)(const void*);
	void (*loop)(const void*);

	// Arduino Client interface API
	int (*connect)(const void*, uint8_t ip[4], uint16_t port);
	uint8_t (*connected)(const void*);
	int (*available)(const void*);
	int (*read)(const void*);
	bool (*write)(const void*, uint8_t* payload, uint8_t pLength);
	void (*flush)(const void*); // wait until all sent
	void (*stop)(const void*);
} Client;

// Constructor
void newClient(Client*, UART_HandleTypeDef* uartUnit, CRC_HandleTypeDef* crcUnit);

// Callbacks
void uartTxCompleteCallback(Client* c);
void uartRxCompleteCallback(Client* c);

#endif /* TCPABSTRACTION_H_ */
