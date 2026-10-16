/*
 * The MIT License (MIT)
 *
 * Copyright (c) 2019 Ha Thach (tinyusb.org)
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in
 * all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
 * THE SOFTWARE.
 *
 */

#include "tusb.h"
#include "usb_func.h"
#include "pico/unique_id.h"

// A distinct PID prevents reusing the template's cached generic-HID driver binding.
#define USB_PID 0x4015

#define USB_VID 0xACDC
#define USB_BCD 0x0200

//--------------------------------------------------------------------+
// Device Descriptors
//--------------------------------------------------------------------+
tusb_desc_device_t const desc_device = {
	.bLength = sizeof(tusb_desc_device_t),
	.bDescriptorType = TUSB_DESC_DEVICE,
	.bcdUSB = USB_BCD,

	// Use Interface Association Descriptor (IAD) for CDC
	// As required by USB Specs IAD's subclass must be common class (2) and
	// protocol must be IAD (1)
	.bDeviceClass = TUSB_CLASS_MISC,
	.bDeviceSubClass = MISC_SUBCLASS_COMMON,
	.bDeviceProtocol = MISC_PROTOCOL_IAD,
	.bMaxPacketSize0 = CFG_TUD_ENDPOINT0_SIZE,

	.idVendor = USB_VID,
	.idProduct = USB_PID,
	.bcdDevice = 0x0100,

	.iManufacturer = 0x01,
	.iProduct = 0x02,
	.iSerialNumber = 0x03,

	.bNumConfigurations = 0x01
};

// Invoked when received GET DEVICE DESCRIPTOR
// Application return pointer to descriptor
uint8_t const* tud_descriptor_device_cb(void) {
	return (uint8_t const*)&desc_device;
}

//--------------------------------------------------------------------+
// HID Report Descriptor
//--------------------------------------------------------------------+
uint8_t const desc_hid_report[] = {
	TUD_HID_REPORT_DESC_KEYBOARD(HID_REPORT_ID(REPORT_KEYBOARD)),
	TUD_HID_REPORT_DESC_MOUSE(HID_REPORT_ID(REPORT_MOUSE))
};

// Invoked when received GET HID REPORT DESCRIPTOR
// Application return pointer to descriptor
// Descriptor contents must exist long enough for transfer to complete
uint8_t const* tud_hid_descriptor_report_cb(uint8_t itf) {
	(void)itf;
	return desc_hid_report;
}

//--------------------------------------------------------------------+
// Configuration Descriptor
//--------------------------------------------------------------------+
enum {
	ITF_NUM_HID = 0,
	ITF_NUM_CDC_0,
	ITF_NUM_CDC_0_DATA,
	ITF_NUM_TOTAL
};
#define CONFIG_TOTAL_LEN (TUD_CONFIG_DESC_LEN + CFG_TUD_CDC * TUD_CDC_DESC_LEN + TUD_HID_DESC_LEN)
#define EPNUM_HID 0x01
#define EPNUM_CDC_0_NOTIF 0x84
#define EPNUM_CDC_0_OUT 0x04
#define EPNUM_CDC_0_IN 0x85

uint8_t const desc_fs_configuration[] = {
	// Config number, interface count, string index, total length, attribute,
	// power in mA
	TUD_CONFIG_DESCRIPTOR(1, ITF_NUM_TOTAL, 0, CONFIG_TOTAL_LEN, 0, 100),

	// Keyboard and mouse share one interrupt IN endpoint, polled every millisecond.
	TUD_HID_DESCRIPTOR(ITF_NUM_HID, 0,
							 HID_ITF_PROTOCOL_NONE, sizeof(desc_hid_report),
							 0x80 | EPNUM_HID,
							 CFG_TUD_HID_EP_BUFSIZE, HID_INTERVAL_MS),
	// 1st CDC: Interface number, string index, EP notification address and
	// size, EP data address (out, in) and size.
	TUD_CDC_DESCRIPTOR(ITF_NUM_CDC_0, 4,
					   EPNUM_CDC_0_NOTIF, 8,
					   EPNUM_CDC_0_OUT, EPNUM_CDC_0_IN, 64)
};

// Invoked when received GET CONFIGURATION DESCRIPTOR
// Application return pointer to descriptor
// Descriptor contents must exist long enough for transfer to complete
uint8_t const* tud_descriptor_configuration_cb(uint8_t index) {
	(void)index; // for multiple configurations

	return desc_fs_configuration;
}

//--------------------------------------------------------------------+
// String Descriptors
//--------------------------------------------------------------------+

// String Descriptor Index
enum {
	STRID_LANGID = 0,
	STRID_MANUFACTURER,
	STRID_PRODUCT,
	STRID_SERIAL,
};

// array of pointer to string descriptors
char const* string_desc_arr[] = {
	(const char[]) {0x09, 0x04}, // 0: is supported language is English (0x0409)
	"Nigh", // 1: Manufacturer
	"TCS3200 Sensor Kit", // 2: Product
	"", // 3: Replaced with the board's unique ID below.
	"TCS3200 Data", // 4: CDC Interface
};

static uint16_t _desc_str[32 + 1];

// Invoked when received GET STRING DESCRIPTOR request
// Application return pointer to descriptor, whose contents must exist long enough for transfer to complete
uint16_t const* tud_descriptor_string_cb(uint8_t index, uint16_t langid) {
	(void)langid;
	size_t chr_count;

	switch(index) {
		case STRID_LANGID:
			memcpy(&_desc_str[1], string_desc_arr[0], 2);
			chr_count = 1;
			break;
		default:
			if(!(index < sizeof(string_desc_arr) / sizeof(string_desc_arr[0]))) return NULL;

			char serial[2 * PICO_UNIQUE_BOARD_ID_SIZE_BYTES + 1];
			const char* str = string_desc_arr[index];
			if(index == STRID_SERIAL) {
				pico_get_unique_board_id_string(serial, sizeof(serial));
				str = serial;
			}

			// Cap at max char
			chr_count = strlen(str);
			size_t const max_count = sizeof(_desc_str) / sizeof(_desc_str[0]) - 1; // -1 for string type
			if(chr_count > max_count) chr_count = max_count;

			// Convert ASCII string into UTF-16
			for(size_t i = 0; i < chr_count; i++) {
				_desc_str[1 + i] = str[i];
			}
			break;
	}

	// first byte is length (including header), second byte is string type
	_desc_str[0] = (uint16_t)((TUSB_DESC_STRING << 8) | (2 * chr_count + 2));

	return _desc_str;
}
