#ifndef TEST_TUSB_H
#define TEST_TUSB_H
#include <stdbool.h>
#include <stdint.h>
typedef enum { HID_REPORT_TYPE_INVALID, HID_REPORT_TYPE_INPUT, HID_REPORT_TYPE_OUTPUT } hid_report_type_t;
typedef struct { uint8_t modifier, reserved, keycode[6]; } hid_keyboard_report_t;
typedef struct { uint8_t buttons; int8_t x, y, wheel, pan; } hid_mouse_report_t;
bool tud_mounted(void);
bool tud_cdc_connected(void);
bool tud_hid_ready(void);
bool tud_hid_report(uint8_t id, const void *report, uint16_t length);
uint32_t tud_cdc_available(void);
uint32_t tud_cdc_read(void *buffer, uint32_t length);
uint32_t tud_cdc_write_available(void);
uint32_t tud_cdc_write(const void *buffer, uint32_t length);
uint32_t tud_cdc_write_flush(void);
void tud_cdc_read_flush(void);
bool tud_cdc_write_clear(void);
void tud_task(void);
bool tusb_init(void);
#endif
