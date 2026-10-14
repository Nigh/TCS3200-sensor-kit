#ifndef SENSOR_USB_H
#define SENSOR_USB_H
#include "protocol.h"
enum { REPORT_KEYBOARD = 1, REPORT_MOUSE = 2 };
void usb_init(void);
void usb_task(void);
void usb_sample(const sample_t *sample);
#endif
