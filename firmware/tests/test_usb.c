#include "usb_func.h"
#include "sensor.h"
#include "tusb.h"
#include <assert.h>
#include <stdio.h>
#include <string.h>

static uint32_t now, write_room = 17;
static bool mounted = true, dtr = true, hid_busy, mock_suspended;
static uint8_t input[PROTOCOL_MAX_FRAME], output[16384], last_hid[9];
static size_t input_length, input_read, output_length;
static uint16_t last_hid_length;
static unsigned hid_reports, boots;
volatile uint32_t sensor_low_hz = 15000, capture_count = 123, capture_drops = 4;
queue_t sample_queue;
uint32_t time_us_32(void) { return now; }
uint64_t time_us_64(void) { return now; }
void reset_usb_boot(uint32_t gpio, uint32_t disable) { (void)gpio; (void)disable; ++boots; }
bool tud_mounted(void) { return mounted; }
bool tud_cdc_connected(void) { return dtr; }
bool tud_hid_ready(void) { return mounted && !hid_busy && !mock_suspended; }
bool tusb_init(void) { return true; }
void tud_task(void) {}
uint32_t tud_cdc_available(void) { return (uint32_t)(input_length - input_read); }
uint32_t tud_cdc_read(void *buffer, uint32_t length) {
    uint32_t remaining = tud_cdc_available();
    if (length > remaining) length = remaining;
    memcpy(buffer, input + input_read, length);
    input_read += length;
    return length;
}
uint32_t tud_cdc_write_available(void) { return write_room; }
uint32_t tud_cdc_write(const void *buffer, uint32_t length) {
    assert(length <= write_room && output_length + length <= sizeof(output));
    // Exercise partial writes, even when the advertised space is larger.
    if (length > 7) length = 7;
    memcpy(output + output_length, buffer, length);
    output_length += length;
    return length;
}
uint32_t tud_cdc_write_flush(void) { return 0; }
void tud_cdc_read_flush(void) { input_read = input_length; }
bool tud_cdc_write_clear(void) { return true; }
bool tud_hid_report(uint8_t id, const void *report, uint16_t length) {
    assert(!hid_busy && length + 1u <= sizeof(last_hid));
    last_hid[0] = id;
    memcpy(last_hid + 1, report, length);
    last_hid_length = length + 1;
    ++hid_reports;
    hid_busy = true;
    return true;
}
void tud_hid_report_complete_cb(uint8_t, const uint8_t *, uint16_t);
void tud_suspend_cb(bool);
void tud_resume_cb(void);
void tud_umount_cb(void);
void tud_mount_cb(void);

static void request(uint8_t type, const uint8_t *payload, uint16_t length) {
    assert(input_read == input_length);
    input_length = protocol_encode(input, type, 17, payload, length);
    input_read = 0;
    usb_task();
}
static unsigned frames;
static uint8_t last_type, last_payload[PROTOCOL_MAX_PAYLOAD];
static uint16_t last_length;
static void response(uint8_t type, uint16_t sequence, const uint8_t *payload,
                     uint16_t length, void *context) {
    (void)context;
    if (type != MSG_SAMPLE) assert(sequence == 17);
    ++frames;
    last_type = type;
    last_length = length;
    memcpy(last_payload, payload, length);
}
static void collect(void) {
    for (unsigned i = 0; i < 1000; ++i) usb_task();
    protocol_parser_t p = {0};
    protocol_feed(&p, output, output_length, now, response, NULL);
    assert(!p.errors && !p.used);
    output_length = 0;
}
static void complete(void) {
    hid_busy = false;
    tud_hid_report_complete_cb(0, last_hid, last_hid_length);
}
static sample_t sample(uint32_t stamp, uint16_t r) {
    return (sample_t){.sequence = stamp / 1000, .timestamp_us = stamp,
        .value = {r, 3000, 4000, 5000}};
}
int main(void) {
    usb_init();
    usb_task();
    uint8_t mode = MODE_TEST;
    request(CMD_MODE, &mode, 1);
    collect();
    assert(last_type == MSG_ACK && last_payload[1] == RESULT_NOT_CONFIGURED);
    uint8_t low[4];
    wire_put32(low, 20000);
    request(CMD_LOW, low, 4);
    collect();
    assert(sensor_low_hz == 20000 && last_payload[1] == RESULT_OK);
    request(CMD_STATUS, NULL, 0);
    collect();
    assert(last_type == MSG_STATUS && last_length == 32);
    assert(wire_u32(last_payload + 8) == 123 && wire_u32(last_payload + 12) == 4);
    request(CMD_MODE, NULL, 0);
    collect();
    assert(last_type == MSG_ACK && last_payload[1] == RESULT_LENGTH);
    request(0x7f, NULL, 0);
    collect();
    assert(last_payload[1] == RESULT_UNKNOWN);
    test_config_t c = {.low_hz = 15000, .reference = {2000, 3000, 4000},
        .enter_tolerance = 10, .exit_threshold = 30, .arm_ms = 1, .depart_ms = 1,
        .hold_ms = 3, .action = ACTION_MOUSE, .mouse_buttons = 1};
    uint8_t config[CONFIG_SIZE];
    config_encode(config, &c);
    request(CMD_CONFIG, config, sizeof(config));
    collect();
    assert(last_payload[1] == RESULT_OK && sensor_low_hz == 15000);
    request(CMD_GET_CONFIG, NULL, 0);
    collect();
    assert(last_type == MSG_CONFIG && last_payload[0] == 1 &&
           !memcmp(last_payload + 1, config, CONFIG_SIZE));
    request(CMD_MODE, &mode, 1);
    collect();
    for (unsigned i = 0; i < 4; ++i) {
        now = i * 1000;
        sample_t s = sample(now, i < 2 ? 2000 : 2030);
        usb_sample(&s);
    }
    usb_task();
    assert(hid_reports == 1 && last_hid[0] == REPORT_MOUSE && last_hid[1] == 1);
    // Closing CDC while the press is in flight must still send a release.
    dtr = false;
    usb_task();
    complete();
    usb_task();
    assert(hid_reports == 2 && last_hid[1] == 0);
    complete();
    dtr = true;
    usb_task();
    output_length = 0;
    // Fill the streaming ring with a stalled host, then ensure a status reply survives.
    write_room = 0;
    for (unsigned i = 0; i < 1000; ++i) {
        sample_t s = sample(i * 1000, 2000);
        usb_sample(&s);
    }
    request(CMD_STATUS, NULL, 0);
    assert(input_read == input_length);
    write_room = 17;
    collect();
    assert(last_type == MSG_STATUS && wire_u32(last_payload + 16) > 0);
    assert(frames > 100);
    // Suspend cancels a delayed action; resume does not restart testing.
    c.delay_ms = 20;
    config_encode(config, &c);
    request(CMD_CONFIG, config, sizeof(config));
    collect();
    request(CMD_MODE, &mode, 1);
    for (unsigned i = 0; i < 4; ++i) {
        now = 10000 + i * 1000;
        sample_t s = sample(now, i < 2 ? 2000 : 2030);
        usb_sample(&s);
    }
    usb_task();
    assert(hid_reports == 2);
    collect();
    mock_suspended = true;
    tud_suspend_cb(false);
    usb_task();
    mock_suspended = false;
    tud_resume_cb();
    now = 34000;
    collect();
    assert(hid_reports == 2);
    request(CMD_STATUS, NULL, 0);
    collect();
    assert(last_payload[28] == MODE_MEASURE);
    mounted = false;
    tud_umount_cb();
    mounted = true;
    tud_mount_cb();
    request(CMD_STATUS, NULL, 0);
    collect();
    assert(last_payload[28] == MODE_MEASURE && last_payload[30] == 1);
    request(CMD_BOOT, NULL, 0);
    collect();
    assert(!boots && last_payload[1] == RESULT_OK);
    now += 250000;
    usb_task();
    assert(boots == 1);
    puts("USB commands, partial writes, stream backpressure, DTR cancellation and HID release checks passed");
}
