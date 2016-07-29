/*
  PubSubClient.cpp - A simple client for MQTT.
  Nick O'Leary
  http://knolleary.net
*/

#include "PubSubClient.h"

// PRIVATE METHODS
// reads a byte into result
static boolean MQTTreadByte(PubSubClient* c, uint8_t * result) {
   uint32_t previousMillis = HAL_GetTick();
   while(!c->_client->available(c->_client)) {
     uint32_t currentMillis = HAL_GetTick();
     if(currentMillis - previousMillis >= ((int32_t) MQTT_SOCKET_TIMEOUT * 1000)){
       return false;
     }
   }
   *result = c->_client->read(c->_client);
   return true;
}

// reads a byte into result[*index] and increments index
static boolean MQTTreadByteFromIndex(PubSubClient* c, uint8_t * result, uint16_t * index){
  uint16_t current_index = *index;
  uint8_t * write_address = &(result[current_index]);
  if(MQTTreadByte(c, write_address)){
    *index = current_index + 1;
    return true;
  }
  return false;
}

static uint16_t MQTTreadPacket(PubSubClient* c, uint8_t* lengthLength) {
    uint16_t len = 0;
    if(!MQTTreadByteFromIndex(c, c->buffer, &len)) return 0;
    bool isPublish = (c->buffer[0]&0xF0) == MQTTPUBLISH;
    uint32_t multiplier = 1;
    uint16_t length = 0;
    uint8_t digit = 0;
    uint16_t skip = 0;
    uint8_t start = 0;

    do {
        if(!MQTTreadByte(c, &digit)) return 0;
        c->buffer[len++] = digit;
        length += (digit & 127) * multiplier;
        multiplier *= 128;
    } while ((digit & 128) != 0);
    *lengthLength = len-1;

    if (isPublish) {
        // Read in topic length to calculate bytes to skip over for Stream writing
        if(!MQTTreadByteFromIndex(c, c->buffer, &len)) return 0;
        if(!MQTTreadByteFromIndex(c, c->buffer, &len)) return 0;
        skip = (c->buffer[*lengthLength+1]<<8)+c->buffer[*lengthLength+2];
        start = 2;
        if (c->buffer[0]&MQTTQOS1) {
            // skip message id
            skip += 2;
        }
    }

    for (uint16_t i = start;i<length;i++) {
        if(!MQTTreadByte(c, &digit)) return 0;
        if (len < MQTT_MAX_PACKET_SIZE) {
            c->buffer[len] = digit;
        }
        len++;
    }

    if (len > MQTT_MAX_PACKET_SIZE) {
        len = 0; // This will cause the packet to be ignored.
    }

    return len;
}

static boolean MQTTwrite(PubSubClient* c, uint8_t header, uint8_t* buf, uint16_t length) {
    uint8_t lenBuf[4];
    uint8_t llen = 0;
    uint8_t digit;
    uint8_t pos = 0;
    uint16_t rc;
    uint16_t len = length;
    do {
        digit = len % 128;
        len = len / 128;
        if (len > 0) {
            digit |= 0x80;
        }
        lenBuf[pos++] = digit;
        llen++;
    } while(len>0);

    buf[4-llen] = header;
    for (int i=0;i<llen;i++) {
        buf[5-llen+i] = lenBuf[i];
    }

#ifdef MQTT_MAX_TRANSFER_SIZE
    uint8_t* writeBuf = buf+(4-llen);
    uint16_t bytesRemaining = length+1+llen;  //Match the length type
    uint8_t bytesToWrite;
    boolean result = true;
    while((bytesRemaining > 0) && result) {
        bytesToWrite = (bytesRemaining > MQTT_MAX_TRANSFER_SIZE)?MQTT_MAX_TRANSFER_SIZE:bytesRemaining;
        rc = c->_client->write(writeBuf,bytesToWrite);
        result = (rc == bytesToWrite);
        bytesRemaining -= rc;
        writeBuf += rc;
    }
    return result;
#else
    rc = c->_client->write(c->_client, buf+(4-llen),length+1+llen);
    c->lastOutActivity = HAL_GetTick();
    return (rc == 1+llen+length);
#endif
}

static uint16_t MQTTwriteString(const char* string, uint8_t* buf, uint16_t pos) {
    const char* idp = string;
    uint16_t i = 0;
    pos += 2;
    while (*idp) {
        buf[pos++] = *idp++;
        i++;
    }
    buf[pos-i-2] = (i >> 8);
    buf[pos-i-1] = (i & 0xFF);
    return pos;
}

// PUBLIC METHODS
static boolean MQTTconnectedPublic(const void* c) {
	PubSubClient* self = (PubSubClient*)c;

    boolean rc;
    if (self->_client == NULL ) {
        rc = false;
    } else {
        rc = (int)self->_client->connected(self->_client);
        if (!rc) {
            if (self->_state == MQTT_CONNECTED) {
            	self->_state = MQTT_CONNECTION_LOST;
            	self->_client->flush(self->_client);
            	self->_client->stop(self->_client);
            }
        }
    }
    return rc;
}

static boolean MQTTconnectPublic(const void* c, const char *id, const char *user, const char *pass, const char* willTopic, uint8_t willQos, boolean willRetain, const char* willMessage)
{
	PubSubClient* self = (PubSubClient*)c;

	if (!MQTTconnectedPublic(self)) {
        int result = 0;

            result = self->_client->connect(self->_client, self->ip, self->port);

        if (result == 1) {
        	self->nextMsgId = 1;
            // Leave room in the buffer for header and variable length field
            uint16_t length = 5;
            unsigned int j;

#if MQTT_VERSION == MQTT_VERSION_3_1
            uint8_t d[9] = {0x00,0x06,'M','Q','I','s','d','p', MQTT_VERSION};
#define MQTT_HEADER_VERSION_LENGTH 9
#elif MQTT_VERSION == MQTT_VERSION_3_1_1
            uint8_t d[7] = {0x00,0x04,'M','Q','T','T',MQTT_VERSION};
#define MQTT_HEADER_VERSION_LENGTH 7
#endif
            for (j = 0;j<MQTT_HEADER_VERSION_LENGTH;j++) {
            	self->buffer[length++] = d[j];
            }

            uint8_t v;
            if (willTopic) {
                v = 0x06|(willQos<<3)|(willRetain<<5);
            } else {
                v = 0x02;
            }

            if(user != NULL) {
                v = v|0x80;

                if(pass != NULL) {
                    v = v|(0x80>>1);
                }
            }

            self->buffer[length++] = v;

            self->buffer[length++] = ((MQTT_KEEPALIVE) >> 8);
            self->buffer[length++] = ((MQTT_KEEPALIVE) & 0xFF);
            length = MQTTwriteString(id,self->buffer,length);
            if (willTopic) {
                length = MQTTwriteString(willTopic,self->buffer,length);
                length = MQTTwriteString(willMessage,self->buffer,length);
            }

            if(user != NULL) {
                length = MQTTwriteString(user,self->buffer,length);
                if(pass != NULL) {
                    length = MQTTwriteString(pass,self->buffer,length);
                }
            }

            MQTTwrite(self, MQTTCONNECT,self->buffer,length-5);

            self->lastInActivity = self->lastOutActivity = HAL_GetTick();

            while (!self->_client->available(self->_client)) {
                unsigned long t = HAL_GetTick();
                if (t-self->lastInActivity >= ((int32_t) MQTT_SOCKET_TIMEOUT*1000UL)) {
                	self->_state = MQTT_CONNECTION_TIMEOUT;
                	self->_client->stop(self->_client);
                    return false;
                }
            }
            uint8_t llen;
            uint16_t len = MQTTreadPacket(self, &llen);

            if (len == 4) {
                if (self->buffer[3] == 0) {
                	self->lastInActivity = HAL_GetTick();
                	self->pingOutstanding = false;
                	self->_state = MQTT_CONNECTED;
                    return true;
                } else {
                	self->_state = self->buffer[3];
                }
            }
            self->_client->stop(self->_client);
        } else {
        	self->_state = MQTT_CONNECT_FAILED;
        }
        return false;
    }
    return true;
}

static boolean MQTTloopPublic(const void* c) {
	PubSubClient* self = (PubSubClient*)c;

    if (MQTTconnectedPublic(self)) {
        unsigned long t = HAL_GetTick();
        if ((t - self->lastInActivity > MQTT_KEEPALIVE*1000UL) || (t - self->lastOutActivity > MQTT_KEEPALIVE*1000UL)) {
            if (self->pingOutstanding) {
            	self->_state = MQTT_CONNECTION_TIMEOUT;
            	self->_client->stop(self->_client);
                return false;
            } else {
            	self->buffer[0] = MQTTPINGREQ;
            	self->buffer[1] = 0;
            	self->_client->write(self->_client, self->buffer,2);
            	self->lastOutActivity = t;
            	self->lastInActivity = t;
            	self->pingOutstanding = true;
            }
        }
        if (self->_client->available(self->_client)) {
            uint8_t llen;
            uint16_t len = MQTTreadPacket(self, &llen);
            uint16_t msgId = 0;
            uint8_t *payload;
            if (len > 0) {
            	self->lastInActivity = t;
                uint8_t type = self->buffer[0]&0xF0;
                if (type == MQTTPUBLISH) {
                    if (self->callback) {
                        uint16_t tl = (self->buffer[llen+1]<<8)+self->buffer[llen+2];
                        char topic[tl+1];
                        for (uint16_t i=0;i<tl;i++) {
                            topic[i] = self->buffer[llen+3+i];
                        }
                        topic[tl] = 0;
                        // msgId only present for QOS>0
                        if ((self->buffer[0]&0x06) == MQTTQOS1) {
                            msgId = (self->buffer[llen+3+tl]<<8)+self->buffer[llen+3+tl+1];
                            payload = self->buffer+llen+3+tl+2;
                            self->callback(topic,payload,len-llen-3-tl-2);

                            self->buffer[0] = MQTTPUBACK;
                            self->buffer[1] = 2;
                            self->buffer[2] = (msgId >> 8);
                            self->buffer[3] = (msgId & 0xFF);
                            self->_client->write(self->_client, self->buffer,4);
                            self->lastOutActivity = t;

                        } else {
                            payload = self->buffer+llen+3+tl;
                            self->callback(topic,payload,len-llen-3-tl);
                        }
                    }
                } else if (type == MQTTPINGREQ) {
                	self->buffer[0] = MQTTPINGRESP;
                	self->buffer[1] = 0;
                	self->_client->write(self->_client, self->buffer,2);
                } else if (type == MQTTPINGRESP) {
                	self->pingOutstanding = false;
                }
            }
        }
        return true;
    }
    return false;
}

static boolean MQTTpublishPublic(const void* c, const char* topic, const uint8_t* payload, unsigned int plength, boolean retained) {
	PubSubClient* self = (PubSubClient*)c;

	if (MQTTconnectedPublic(self)) {
        if (MQTT_MAX_PACKET_SIZE < 5 + 2+strlen(topic) + plength) {
            // Too long
            return false;
        }
        // Leave room in the buffer for header and variable length field
        uint16_t length = 5;HAL_GPIO_TogglePin(GPIOA, GPIO_PIN_5);
        length = MQTTwriteString(topic,self->buffer,length);
        uint16_t i;
        for (i=0;i<plength;i++) {
        	self->buffer[length++] = payload[i];
        }
        uint8_t header = MQTTPUBLISH;
        if (retained) {
            header |= 1;
        }
        return MQTTwrite(self, header,self->buffer,length-5);
    }
    return false;
}



static boolean MQTTsubscribe(const void* c, const char* topic, uint8_t qos) {
	PubSubClient* self = (PubSubClient*)c;

    if (qos < 0 || qos > 1) {
        return false;
    }
    if (MQTT_MAX_PACKET_SIZE < 9 + strlen(topic)) {
        // Too long
        return false;
    }
    if (MQTTconnectedPublic(self)) {
        // Leave room in the buffer for header and variable length field
        uint16_t length = 5;
        self->nextMsgId++;
        if (self->nextMsgId == 0) {
        	self->nextMsgId = 1;
        }
        self->buffer[length++] = (self->nextMsgId >> 8);
        self->buffer[length++] = (self->nextMsgId & 0xFF);
        length = MQTTwriteString((char*)topic, self->buffer,length);
        self->buffer[length++] = qos;
        return MQTTwrite(self, MQTTSUBSCRIBE|MQTTQOS1,self->buffer,length-5);
    }
    return false;
}

static boolean MQTTunsubscribe(const void* c, const char* topic) {
	PubSubClient* self = (PubSubClient*)c;

    if (MQTT_MAX_PACKET_SIZE < 9 + strlen(topic)) {
        // Too long
        return false;
    }
    if (MQTTconnectedPublic(self)) {
        uint16_t length = 5;
        self->nextMsgId++;
        if (self->nextMsgId == 0) {
        	self->nextMsgId = 1;
        }
        self->buffer[length++] = (self->nextMsgId >> 8);
        self->buffer[length++] = (self->nextMsgId & 0xFF);
        length = MQTTwriteString(topic, self->buffer,length);
        return MQTTwrite(self, MQTTUNSUBSCRIBE|MQTTQOS1,self->buffer, length-5);
    }
    return false;
}

static void MQTTdisconnect(void const* c) {
	PubSubClient* self = (PubSubClient*)c;

	self->buffer[0] = MQTTDISCONNECT;
	self->buffer[1] = 0;
	self->_client->write(self->_client, self->buffer,2);
	self->_state = MQTT_DISCONNECTED;
	self->_client->stop(self->_client);
	self->lastInActivity = self->lastOutActivity = HAL_GetTick();
}

static int MQTTstate(const void* c) {
    return ((PubSubClient*)c)->_state;
}

// Constructor
void newPubSubClient(PubSubClient* c, uint8_t *ip, uint16_t port, MQTT_CALLBACK_SIGNATURE, Client* client)
{
	c->connect = MQTTconnectPublic;
	c->disconnect = MQTTdisconnect;
	c->publish = MQTTpublishPublic;
	c->subscribe = MQTTsubscribe;
	c->unsubscribe = MQTTunsubscribe;
	c->loop = MQTTloopPublic;
	c->connected = MQTTconnectedPublic;
	c->state = MQTTstate;

	c->ip[0] = ip[0]; c->ip[1] = ip[1]; c->ip[2] = ip[2]; c->ip[3] = ip[3];
	c->port = port;
	c->_client = client;
	c->callback = callback;

	c->_state = MQTT_DISCONNECTED;
}
