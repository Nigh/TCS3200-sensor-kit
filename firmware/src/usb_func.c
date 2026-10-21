#include "usb_func.h"
#include "app.h"
#include "sensor.h"
#include "pico/bootrom.h"
#include "pico/stdlib.h"
#include "tusb.h"
#include <string.h>

#define TX_CAPACITY 4096u
static uint8_t tx[TX_CAPACITY];
static uint32_t tx_read, tx_write, stream_drops;
static protocol_parser_t parser;
static app_t app;
static bool suspended, was_connected, boot_pending;
static uint32_t boot_deadline;
static hid_keyboard_report_t keyboard_report;
static hid_mouse_report_t mouse_report;

static bool connected(void) {
    return tud_mounted() && !suspended && tud_cdc_connected();
}
static uint32_t tx_free(void) {
    return TX_CAPACITY - (tx_write - tx_read);
}
static void send_frame(uint8_t type, uint16_t sequence, const uint8_t *data, uint16_t length) {
    uint8_t frame[PROTOCOL_MAX_FRAME];
    size_t count = protocol_encode(frame, type, sequence, data, length);
    for (size_t i = 0; i < count; ++i)
        tx[(tx_write++) & (TX_CAPACITY - 1u)] = frame[i];
}
static void drain(void) {
    if (!connected())
        return;
    if (tx_read != tx_write) {
        uint32_t count = tx_write - tx_read;
        uint32_t contiguous = TX_CAPACITY - (tx_read & (TX_CAPACITY - 1u));
        if (count > contiguous)
            count = contiguous;
        uint32_t available = tud_cdc_write_available();
        if (count > available)
            count = available;
        if (count)
            tx_read += tud_cdc_write(tx + (tx_read & (TX_CAPACITY - 1u)), count);
    }
    tud_cdc_write_flush();
}
static void command(uint8_t type, uint16_t sequence, const uint8_t *data, uint16_t length, void *context) {
    (void)context;
    uint8_t result = RESULT_OK;
    if (boot_pending) {
        result = RESULT_BUSY;
    } else if (type == CMD_STATUS) {
        if (length) {
            result = RESULT_LENGTH;
        } else {
            uint8_t status[32] = {0};
            uint32_t low_hz = sensor_low_hz;
            wire_put16(status, sample_mask_rate_hz(low_hz, sensor_channels));
            wire_put16(status + 2, (uint16_t)sample_channel_slot_us(low_hz));
            wire_put32(status + 4, low_hz);
            wire_put32(status + 8, capture_count);
            wire_put32(status + 12, capture_drops);
            wire_put32(status + 16, stream_drops);
            wire_put32(status + 20, parser.errors);
            wire_put32(status + 24, (uint32_t)(time_us_64() / 1000u));
            status[28] = app.mode;
            status[29] = app.state;
            status[30] = app.configured;
            status[31] = sensor_channels;
            send_frame(MSG_STATUS, sequence, status, sizeof(status));
            return;
        }
    } else if (type == CMD_USB_STATUS) {
        if (length) {
            result = RESULT_LENGTH;
        } else {
            uint8_t status[4];
            wire_put16(status, HID_INTERVAL_MS * 1000u);
            wire_put16(status + 2, usb_poll_rate());
            send_frame(MSG_USB_STATUS, sequence, status, sizeof(status));
            return;
        }
    } else if (type == CMD_MODE) {
        result = length == 1 ? app_set_mode(&app, data[0], time_us_32()) : RESULT_LENGTH;
    } else if (type == CMD_CONFIG) {
        if (length != CONFIG_SIZE) {
            result = RESULT_LENGTH;
        } else {
            test_config_t config;
            config_decode(&config, data);
            if (!app_configure(&app, &config))
                result = RESULT_VALUE;
            else {
                sensor_low_hz = config.low_hz;
                sensor_channels = config.channels;
            }
        }
    } else if (type == CMD_GET_CONFIG) {
        if (length) {
            result = RESULT_LENGTH;
        } else {
            uint8_t config[CONFIG_SIZE + 1];
            config[0] = app.configured;
            config_encode(config + 1, &app.config);
            send_frame(MSG_CONFIG, sequence, config, sizeof(config));
            return;
        }
    } else if (type == CMD_CHANNELS) {
        if (length != 1)
            result = RESULT_LENGTH;
        else if (data[0] > 15)
            result = RESULT_VALUE;
        else if (app.mode != MODE_MEASURE || app.state != STATE_WAIT)
            result = RESULT_BUSY;
        else {
            sensor_channels = data[0];
            app.configured = false;
        }
    } else if (type == CMD_LOW) {
        if (length != 4)
            result = RESULT_LENGTH;
        else if (wire_u32(data) < SENSOR_LOW_MIN_HZ || wire_u32(data) > SENSOR_LOW_MAX_HZ)
            result = RESULT_VALUE;
        else if (app.mode != MODE_MEASURE || app.state != STATE_WAIT)
            result = RESULT_BUSY;
        else {
            sensor_low_hz = wire_u32(data);
            app.config.low_hz = sensor_low_hz;
            app.configured = false;
        }
    } else if (type == CMD_BOOT) {
        if (length)
            result = RESULT_LENGTH;
        else if (app.mode != MODE_MEASURE || app.state != STATE_WAIT)
            result = RESULT_BUSY;
        else {
            boot_pending = true;
            boot_deadline = time_us_32() + 250000u;
        }
    } else {
        result = RESULT_UNKNOWN;
    }
    uint8_t ack[] = {type, result};
    send_frame(MSG_ACK, sequence, ack, sizeof(ack));
}
static void hid_task(void) {
    test_config_t action;
    bool press;
    app_poll(&app, time_us_32());
    if (!tud_hid_ready() || !app_hid_request(&app, &action, &press))
        return;
    bool sent;
    if (action.action == ACTION_KEYBOARD) {
        hid_keyboard_report_t report = {0};
        if (press) {
            report.modifier = action.modifiers;
            report.keycode[0] = action.keycode;
        }
        sent = tud_hid_report(REPORT_KEYBOARD, &report, sizeof(report));
        if (sent)
            keyboard_report = report;
    } else {
        hid_mouse_report_t report = {0};
        report.buttons = press ? action.mouse_buttons : 0;
        sent = tud_hid_report(REPORT_MOUSE, &report, sizeof(report));
        if (sent)
            mouse_report = report;
    }
    if (sent) {
        usb_poll_reset();
        app_hid_submitted(&app);
    }
}
void usb_init(void) {
    app_init(&app);
    tusb_init();
}
void usb_task(void) {
    tud_task();
    bool live = connected();
    if (!live && was_connected) {
        app_set_mode(&app, MODE_MEASURE, time_us_32());
        tx_read = tx_write = 0;
        parser.used = 0;
        tud_cdc_write_clear();
        tud_cdc_read_flush();
    }
    was_connected = live;
    usb_poll_task(live && tud_hid_ready() && !app.in_flight);
    hid_task();
    drain();
    // One byte at a time reserves room for every command reply without blocking.
    for (unsigned i = 0; live && i < 128 && tud_cdc_available() && tx_free() >= PROTOCOL_MAX_FRAME; ++i) {
        uint8_t byte;
        if (tud_cdc_read(&byte, 1) == 1)
            protocol_feed(&parser, &byte, 1, time_us_32(), command, NULL);
    }
    protocol_feed(&parser, NULL, 0, time_us_32(), command, NULL);
    hid_task();
    if (boot_pending && (int32_t)(time_us_32() - boot_deadline) >= 0)
        reset_usb_boot(0, 0);
}
void usb_sample(const sample_t *s) {
    app_sample(&app, s);
    hid_task();
    if (!connected() || boot_pending)
        return;
    // Leave a complete maximum-sized reply slot even when streaming is congested.
    if (tx_free() < SAMPLE_SIZE + PROTOCOL_OVERHEAD + PROTOCOL_MAX_FRAME) {
        ++stream_drops;
        return;
    }
    uint8_t payload[SAMPLE_SIZE] = {0};
    wire_put32(payload, s->sequence);
    wire_put32(payload + 4, s->timestamp_us);
    for (unsigned i = 0; i < 4; ++i)
        wire_put32(payload + 8 + 4 * i, s->value[i]);
    payload[24] = s->low_mask;
    payload[25] = s->clip_mask;
    payload[26] = app.mode;
    payload[27] = app.state;
    payload[28] = s->flags;
    payload[29] = s->channels;
    send_frame(MSG_SAMPLE, (uint16_t)s->sequence, payload, sizeof(payload));
}
static void disconnected(void) {
    usb_poll_reset();
    app_disconnect(&app);
    memset(&keyboard_report, 0, sizeof(keyboard_report));
    memset(&mouse_report, 0, sizeof(mouse_report));
    tx_read = tx_write = 0;
    parser.used = 0;
    was_connected = false;
    boot_pending = false;
}
void tud_mount_cb(void) {
    suspended = false;
    disconnected();
}
void tud_umount_cb(void) {
    disconnected();
}
void tud_suspend_cb(bool remote_wakeup_en) {
    (void)remote_wakeup_en;
    suspended = true;
    app_set_mode(&app, MODE_MEASURE, time_us_32());
}
void tud_resume_cb(void) {
    suspended = false;
}
void tud_hid_report_complete_cb(uint8_t instance, const uint8_t *report, uint16_t length) {
    (void)instance;
    (void)report;
    (void)length;
    app_hid_complete(&app, time_us_32());
}
void tud_hid_report_failed_cb(uint8_t instance, hid_report_type_t type, const uint8_t *report, uint16_t length) {
    (void)instance;
    (void)type;
    (void)report;
    (void)length;
    app_hid_failed(&app);
}
uint16_t tud_hid_get_report_cb(uint8_t instance, uint8_t report_id, hid_report_type_t type,
                              uint8_t *buffer, uint16_t requested) {
    (void)instance;
    if (type != HID_REPORT_TYPE_INPUT)
        return 0;
    const void *report;
    uint16_t length;
    if (report_id == REPORT_KEYBOARD) {
        report = &keyboard_report;
        length = sizeof(keyboard_report);
    } else if (report_id == REPORT_MOUSE) {
        report = &mouse_report;
        length = sizeof(mouse_report);
    } else {
        return 0;
    }
    if (length > requested)
        length = requested;
    memcpy(buffer, report, length);
    return length;
}
void tud_hid_set_report_cb(uint8_t instance, uint8_t report_id, hid_report_type_t type,
                          const uint8_t *buffer, uint16_t length) {
    // Keyboard LED output reports need no action on this board.
    (void)instance;
    (void)report_id;
    (void)type;
    (void)buffer;
    (void)length;
}
