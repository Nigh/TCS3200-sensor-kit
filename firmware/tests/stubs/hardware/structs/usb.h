#ifndef TEST_USB_HW_H
#define TEST_USB_HW_H
#include <stdint.h>
typedef struct { volatile uint32_t ep_nak_stall_status; } usb_hw_t;
extern usb_hw_t *usb_hw;
#endif
