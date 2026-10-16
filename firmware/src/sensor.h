#ifndef SENSOR_CAPTURE_H
#define SENSOR_CAPTURE_H
#include "protocol.h"
#include "pico/util/queue.h"
#define SENSOR_OUT_PIN 0u
#define SENSOR_S2_PIN 1u
#define SENSOR_S3_PIN 2u
#define SENSOR_S0_PIN 3u
#define SENSOR_S1_PIN 4u
#define SENSOR_OE_PIN 5u
extern queue_t sample_queue;
extern volatile uint32_t sensor_low_hz, capture_count, capture_drops;
void sensor_init(void);
void sensor_run(void);
#endif
