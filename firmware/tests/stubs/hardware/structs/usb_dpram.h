#ifndef TEST_USB_DPRAM_H
#define TEST_USB_DPRAM_H
#include <stdint.h>
#define EP_CTRL_ENABLE_BITS (1u << 31)
#define EP_CTRL_INTERRUPT_ON_NAK (1u << 16)
typedef struct { struct { volatile uint32_t in, out; } ep_ctrl[15]; } usb_dpram_t;
extern usb_dpram_t *usb_dpram;
#endif
