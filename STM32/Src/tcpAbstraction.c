/*
 * tcpAbstraction.c
 *
 *  Created on: Jul 9, 2016
 *      Author: Roan
 */
#include "tcpAbstraction.h"

// Private Methods
static bool rx_available(Client* c)
{
  return (c->pRx_rx != c->pRead_rx) || c->rxFull;
}

static bool rx_read(Client* c, uint8_t* result)
{
  uint32_t start = HAL_GetTick();
  while (!rx_available((Client*)c))
  {
    uint32_t now  = HAL_GetTick();
    if ((now - start) >= SERIAL_TIMEOUT)
      return false;
  }
  *result = c->rxBuffer[c->pRead_rx++];
  c->rxFull = false;
  return true;
}

static bool readPacket(Client* c)
{
  // Length byte
  if (!rx_read(c, c->workBuffer))
	  return false;

  // Command byte
  if (!rx_read(c, &c->workBuffer[1]))
	  return false;

  // Payload + CRC32
  uint8_t i;
  for (i = 2; i <= c->workBuffer[0]; i++)
  {
    if (!rx_read(c, &c->workBuffer[i]))
    	return false;
  }

  // Integrity checking
  uint32_t crc = c->workBuffer[i-4] | (c->workBuffer[i-3] << 8) | (c->workBuffer[i-2] << 16) | (c->workBuffer[i-1] << 24);
  uint32_t crcCode = HAL_CRC_Calculate(c->peripheral_CRC, (uint32_t*)(c->workBuffer), i - 4);
  crcCode = crcCode ^ 0xffffffff;
  if (crc != crcCode)
	  return false;

  return true;
}

static bool writePacket(Client* c, uint8_t command, uint8_t* payload, uint8_t pLength)
{
  c->workBuffer[0] = pLength + 5;
  c->workBuffer[1] = command;
  if (payload != NULL)
  {
    uint8_t i;
    for (i = 2; i < pLength + 2; i++)
    {
    	c->workBuffer[i] = payload[i-2];
    }
  }
  uint32_t crcCode = HAL_CRC_Calculate(c->peripheral_CRC, (uint32_t*)(c->workBuffer), pLength + 2);
  crcCode = crcCode ^ 0xffffffff;
  c->workBuffer[pLength + 2] = crcCode & 0x000000FF;
  c->workBuffer[pLength + 3] = (crcCode & 0x0000FF00) >> 8;
  c->workBuffer[pLength + 4] = (crcCode & 0x00FF0000) >> 16;
  c->workBuffer[pLength + 5] = (crcCode & 0xFF000000) >> 24;
  while (!c->txReady) {}
  HAL_UART_Transmit_IT(c->peripheral_UART, c->workBuffer, pLength + 6);
  c->txReady = false;
  return true;
}

static bool publish(Client* c, uint8_t* payload, uint8_t pLength)
{
  if (c->ackOutstanding)
	  return false;

  c->ackOutstanding = true;

  uint8_t cmdSequence = PROTOCOL_PUBLISH;
  if (c->sequenceTxFlag)
  {
    cmdSequence |= 0x80;
  }
  /* NOT NECESSARY AS WILL RESULT IN 0 ANYWAY
  else
  {
    cmdSequence &= 0x7F;
  }*/
  if (!writePacket(((Client*)c), cmdSequence, payload, pLength))
	  return false;

  return true;
}

static bool writeByte(Client* c, uint8_t* byte)
{
	return publish(c, byte, 1);
}

// Public Methods
static int availablePublic(const void* c)
{
	return (((Client*)c)->pRx_read != ((Client*)c)->pRead_read) || ((Client*)c)->readFull;
}

static int readPublic(const void* c)
{
  if (!((Client*)c)->available(c))
    return -1;

  uint8_t ch = ((Client*)c)->readBuffer[((Client*)c)->pRead_read++];
  ((Client*)c)->readFull = false;
  return ch;
}

static int connectPublic(const void* c, uint8_t ip[4], uint16_t port)
{
  if (((Client*)c)->start(c))
    return 1;

  return -1;

  /*SUCCESS 1
  TIMED_OUT -1
  INVALID_SERVER -2
  TRUNCATED -3
  INVALID_RESPONSE -4
  */
}
bool first = false;
static uint8_t connectedPublic(const void* c)
{
	if (!first)
	{
		first = true;
		return false;
	}
  return true;
}

static void flushPublic(const void* c)
{
  while (!((Client*)c)->txReady);
}

static void stopPublic(const void* c)
{
  ;
}

static bool writePublic(const void* c, uint8_t* payload, uint8_t pLength)
{
	return publish(((Client*)c), payload, pLength);
}

static bool startPublic(const void* c)
{
	HAL_UART_Receive_IT(((Client*)c)->peripheral_UART, &((Client*)c)->rxByte, 1);
	return true;
}

static void loopPublic(const void* c)
{
	if (rx_available((Client*)c))
	{
		if (readPacket((Client*)c))
		{
			bool rxSeqFlag = (((Client*)c)->workBuffer[1] & 0x80) > 0;
			switch (((Client*)c)->workBuffer[1] & 0x7F)
			{
				// Message from PC
				case PROTOCOL_PUBLISH:
				if (rxSeqFlag == ((Client*)c)->expectedRxSeqFlag)
				{
					((Client*)c)->expectedRxSeqFlag = !((Client*)c)->expectedRxSeqFlag;

          if (((Client*)c)->workBuffer[0] > 5)
          {
            for (uint8_t i = 0; i < ((Client*)c)->workBuffer[0] - 5; i++)
            {
            	((Client*)c)->readBuffer[((Client*)c)->pRx_read++] = ((Client*)c)->workBuffer[2 + i];
            }
            ((Client*)c)->readFull = (((Client*)c)->pRead_read == ((Client*)c)->pRx_read);
          }

						// DEBUG LED
						if (((Client*)c)->workBuffer[2] == 0x31)
						{
							HAL_GPIO_WritePin(GPIOA, GPIO_PIN_5, GPIO_PIN_SET);
						} else if (((Client*)c)->workBuffer[2] == 0x32)
						{
							HAL_GPIO_WritePin(GPIOA, GPIO_PIN_5, GPIO_PIN_RESET);
						}
				}
				writePacket(((Client*)c), PROTOCOL_ACK | (((Client*)c)->workBuffer[1] & 0x80), NULL, 0);
				break;

				// ACK from PC
				case PROTOCOL_ACK:
				if (rxSeqFlag == ((Client*)c)->sequenceTxFlag)
				{
					((Client*)c)->sequenceTxFlag = !((Client*)c)->sequenceTxFlag;
					((Client*)c)->ackOutstanding = false;
				}
				break;
			}
		}
	}
}

// Callbacks
void uartTxCompleteCallback(Client* c)
{
	c->txReady = true;
}

void uartRxCompleteCallback(Client* c)
{
	c->rxBuffer[c->pRx_rx++] = c->rxByte;
	c->rxFull = (c->pRead_rx == c->pRx_rx);
	HAL_UART_Receive_IT(c->peripheral_UART, &c->rxByte, 1);
}

// Constructor
void newClient(Client* c, UART_HandleTypeDef* uartUnit, CRC_HandleTypeDef* crcUnit)
{
	c->peripheral_UART = uartUnit;
	c->peripheral_CRC = crcUnit;

	c->pRx_rx = 0;
	c->pRead_rx = 0;
	c->pRx_read = 0;
	c->pRead_read = 0;
	c->rxFull = false;
	c->readFull = false;
	c->txReady = true;
	c->ackOutstanding = false;
	c->sequenceTxFlag = false;
	c->expectedRxSeqFlag = false;

	c->start = startPublic;
	c->loop = loopPublic;

	c->connect = connectPublic;
	c->connected = connectedPublic;
	c->available = availablePublic;
	c->read = readPublic;
	c->write = writePublic;
	c->flush = flushPublic;
	c->stop = stopPublic;
}
