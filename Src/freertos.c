/**
  ******************************************************************************
  * File Name          : freertos.c
  * Description        : Code for freertos applications
  ******************************************************************************
  *
  * COPYRIGHT(c) 2016 STMicroelectronics
  *
  * Redistribution and use in source and binary forms, with or without modification,
  * are permitted provided that the following conditions are met:
  *   1. Redistributions of source code must retain the above copyright notice,
  *      this list of conditions and the following disclaimer.
  *   2. Redistributions in binary form must reproduce the above copyright notice,
  *      this list of conditions and the following disclaimer in the documentation
  *      and/or other materials provided with the distribution.
  *   3. Neither the name of STMicroelectronics nor the names of its contributors
  *      may be used to endorse or promote products derived from this software
  *      without specific prior written permission.
  *
  * THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
  * AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
  * IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
  * DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
  * FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
  * DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
  * SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
  * CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
  * OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
  * OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
  *
  ******************************************************************************
  */

/* Includes ------------------------------------------------------------------*/
#include "FreeRTOS.h"
#include "task.h"
#include "cmsis_os.h"

/* USER CODE BEGIN Includes */
#include <string.h>
#include "gpio.h"
#include "usart.h"
#include "crc.h"
#include "PubSubClient.h"
/* USER CODE END Includes */

/* Variables -----------------------------------------------------------------*/
osThreadId defaultTaskHandle;

/* USER CODE BEGIN Variables */
Client connection;
PubSubClient mqttConnection;
bool sendPackToPC;
const char* topic = "stm32f334";
const char* pcMessage = "Hello i5! I am a STM32F3!\n";

/* USER CODE END Variables */

/* Function prototypes -------------------------------------------------------*/
void StartDefaultTask(void const * argument);

void MX_FREERTOS_Init(void); /* (MISRA C 2004 rule 8.1) */

/* USER CODE BEGIN FunctionPrototypes */

/* USER CODE END FunctionPrototypes */

/* Hook prototypes */
void HAL_UART_TxCpltCallback(UART_HandleTypeDef *huart)
{
	uartTxCompleteCallback(&connection);
}

void HAL_UART_RxCpltCallback(UART_HandleTypeDef *huart)
{
	uartRxCompleteCallback(&connection);
}

void HAL_GPIO_EXTI_Callback(uint16_t GPIO_Pin)
{
  static uint32_t debounce;
  if (GPIO_Pin == GPIO_PIN_13)
  {
    uint32_t now = HAL_GetTick();
    if (now - debounce > 200)
    {
      sendPackToPC = true;
      debounce = now;
    }
  }
}

void MQTTCallbek()
{

}

/* Init FreeRTOS */

void MX_FREERTOS_Init(void) {
  /* USER CODE BEGIN Init */
       
  /* USER CODE END Init */

  /* USER CODE BEGIN RTOS_MUTEX */
  /* add mutexes, ... */
  /* USER CODE END RTOS_MUTEX */

  /* USER CODE BEGIN RTOS_SEMAPHORES */
  /* add semaphores, ... */
  /* USER CODE END RTOS_SEMAPHORES */

  /* USER CODE BEGIN RTOS_TIMERS */
  /* start timers, add new ones, ... */
  /* USER CODE END RTOS_TIMERS */

  /* Create the thread(s) */
  /* definition and creation of defaultTask */
  osThreadDef(defaultTask, StartDefaultTask, osPriorityNormal, 0, 128);
  defaultTaskHandle = osThreadCreate(osThread(defaultTask), NULL);

  /* USER CODE BEGIN RTOS_THREADS */
  /* add threads, ... */
  /* USER CODE END RTOS_THREADS */

  /* USER CODE BEGIN RTOS_QUEUES */
  /* add queues, ... */
  /* USER CODE END RTOS_QUEUES */
}

/* StartDefaultTask function */
void StartDefaultTask(void const * argument)
{

  /* USER CODE BEGIN StartDefaultTask */
	const char* id = "stm32BABY";
	const char* user = "";
	const char* pass = "";
	const char* willTopic = "";
	const char* willMsg = "";

	newClient(&connection, &huart2, &hcrc);
	//connection.start(&connection);
	uint8_t address[4] = {127, 0, 0, 1};
	newPubSubClient(&mqttConnection, address, 5511, MQTTCallbek, &connection);
	bool connectedAfter = false;
  /* Infinite loop */
	for(;;)
	{
		connection.loop(&connection);

		if (sendPackToPC)
		{
			///HAL_GPIO_TogglePin(GPIOA, GPIO_PIN_5);
			sendPackToPC = false;
			// const char *id, const char *user, const char *pass, const char* willTopic, uint8_t willQos, boolean willRetain, const char* willMessage);
			mqttConnection.publish(&mqttConnection, topic, (const uint8_t*)pcMessage, strlen(pcMessage), false);

			//if (!connection.write(&connection, (uint8_t*)pcMessage, strlen(pcMessage)))
				//HAL_GPIO_TogglePin(GPIOA, GPIO_PIN_5);
		}
		if (!connectedAfter)
		{
			connectedAfter = true;
			mqttConnection.connect(&mqttConnection, id, user, pass, willTopic, 0, false, willMsg);
		}
		mqttConnection.loop(&mqttConnection);
	}
  /* USER CODE END StartDefaultTask */
}

/* USER CODE BEGIN Application */
     
/* USER CODE END Application */

/************************ (C) COPYRIGHT STMicroelectronics *****END OF FILE****/
