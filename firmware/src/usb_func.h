#ifndef SENSOR_USB_H
#define SENSOR_USB_H
#include "protocol.h"
#define HID_INTERVAL_MS 1u
void usb_poll_task(bool idle);
void usb_poll_reset(void);
uint16_t usb_poll_rate(void);
enum { REPORT_KEYBOARD = 1, REPORT_MOUSE = 2 };
void usb_init(void);
void usb_task(void);
void usb_sample(const sample_t *sample);
#endif
