#include "app.h"
#include <assert.h>
#include <stdio.h>
#include <string.h>

static uint8_t expected_payload[PROTOCOL_MAX_PAYLOAD];
static uint16_t expected_length;
static unsigned received;
static void receive(uint8_t type, uint16_t sequence, const uint8_t *payload,
                    uint16_t length, void *context) {
    assert(context == &received);
    assert(type == CMD_CONFIG && sequence == 0x1234);
    assert(length == expected_length);
    assert(memcmp(payload, expected_payload, length) == 0);
    ++received;
}
static void test_protocol(void) {
    assert(protocol_crc((const uint8_t *)"123456789", 9) == 0x29b1);
    uint8_t frame[PROTOCOL_MAX_FRAME];
    const uint8_t status_vector[] = {0x54, 0x43, 2, 1, 1, 0, 0, 0, 0xb5, 0x59};
    assert(protocol_encode(frame, CMD_STATUS, 1, NULL, 0) == sizeof(status_vector));
    assert(!memcmp(frame, status_vector, sizeof(status_vector)));
    protocol_parser_t p = {0};
    for (uint16_t length = 0; length <= PROTOCOL_MAX_PAYLOAD; ++length) {
        expected_length = length;
        for (unsigned i = 0; i < length; ++i)
            expected_payload[i] = (uint8_t)(i * 37u);
        size_t count = protocol_encode(frame, CMD_CONFIG, 0x1234, expected_payload, length);
        assert(count == length + PROTOCOL_OVERHEAD);
        for (size_t split = 0; split <= count; ++split) {
            p.used = 0;
            unsigned before = received;
            protocol_feed(&p, frame, split, 1, receive, &received);
            protocol_feed(&p, frame + split, count - split, 2, receive, &received);
            assert(received == before + 1 && !p.used);
        }
    }
    assert(!protocol_encode(frame, 0, 0, expected_payload, PROTOCOL_MAX_PAYLOAD + 1));
    expected_length = 0;
    size_t count = protocol_encode(frame, CMD_CONFIG, 0x1234, NULL, 0);
    uint8_t broken[PROTOCOL_MAX_FRAME];
    memcpy(broken, frame, count);
    broken[count - 1] ^= 1;
    unsigned before = received;
    protocol_feed(&p, broken, count, 3, receive, &received);
    assert(received == before && p.errors);
    protocol_feed(&p, frame, count, 4, receive, &received);
    assert(received == before + 1);
    memcpy(broken, frame, count);
    broken[6] = 65;
    protocol_feed(&p, broken, count, 5, receive, &received);
    protocol_feed(&p, frame, count, 6, receive, &received);
    assert(received == before + 2);
    protocol_feed(&p, frame, 8, 7, receive, &received);
    protocol_feed(&p, NULL, 0, 100007, receive, &received);
    assert(!p.used);
    protocol_feed(&p, frame, count, 100008, receive, &received);
    assert(received == before + 3);
    // Arbitrary garbage exercises bounded resynchronization under sanitizers.
    uint32_t random = 123;
    for (unsigned i = 0; i < 100000; ++i) {
        random = random * 1664525u + 1013904223u;
        uint8_t byte = (uint8_t)(random >> 24);
        protocol_feed(&p, &byte, 1, 100009, receive, &received);
    }
}
static test_config_t config(void) {
    return (test_config_t){.low_hz = 20000, .reference = {2000, 3000, 4000},
        .enter_tolerance = 10, .exit_threshold = 30, .arm_ms = 2, .depart_ms = 2,
        .delay_ms = 5, .hold_ms = 3, .action = ACTION_KEYBOARD, .keycode = 4};
}
static sample_t sample(uint32_t time, int difference) {
    return (sample_t){.timestamp_us = time,
        .value = {(uint16_t)(2000 + difference), 3000, 4000, 5000}};
}
static void arm_and_depart(app_t *a, uint32_t start) {
    for (unsigned i = 0; i < 3; ++i) {
        sample_t s = sample(start + i * 1000u, 0);
        app_sample(a, &s);
    }
    assert(a->state == STATE_ARM);
    for (unsigned i = 3; i < 6; ++i) {
        sample_t s = sample(start + i * 1000u, 30);
        app_sample(a, &s);
    }
    assert(a->state == STATE_DELAY);
}
static void test_app(void) {
    app_t a;
    app_init(&a);
    assert(app_set_mode(&a, MODE_TEST, 0) == RESULT_NOT_CONFIGURED);
    test_config_t c = config(), decoded;
    uint8_t bytes[CONFIG_SIZE], again[CONFIG_SIZE];
    config_encode(bytes, &c);
    config_decode(&decoded, bytes);
    config_encode(again, &decoded);
    assert(!memcmp(bytes, again, CONFIG_SIZE));
    assert(config_valid(&decoded) && app_configure(&a, &decoded));
    c.exit_threshold = c.enter_tolerance;
    assert(!app_configure(&a, &c) && a.config.exit_threshold == 30);
    c = config();
    c.action = ACTION_MOUSE;
    assert(!config_valid(&c));
    c.keycode = 0;
    c.mouse_buttons = 31;
    assert(config_valid(&c));
    c.delay_ms = 60001;
    assert(!config_valid(&c));
    c = config();
    assert(app_set_mode(&a, 2, 0) == RESULT_VALUE);
    assert(app_set_mode(&a, MODE_TEST, 1000) == RESULT_OK);
    sample_t s = sample(0, 0);
    app_sample(&a, &s);
    assert(!a.confirming); // A queued sample from before mode selection is ignored.
    s = sample(1000, 0);
    app_sample(&a, &s);
    s = sample(3000, 0);
    app_sample(&a, &s);
    assert(a.state == STATE_WAIT && !a.confirming); // A gap cannot satisfy confirmation.
    arm_and_depart(&a, 4000);
    app_poll(&a, 13999);
    assert(a.state == STATE_DELAY);
    app_poll(&a, 14000);
    test_config_t action;
    bool press;
    assert(app_hid_request(&a, &action, &press) && press && action.keycode == 4);
    app_hid_submitted(&a);
    assert(!app_hid_request(&a, &action, &press));
    app_hid_complete(&a, 15000);
    app_poll(&a, 17999);
    assert(a.state == STATE_HOLD);
    app_poll(&a, 18000);
    assert(app_hid_request(&a, &action, &press) && !press);
    app_hid_submitted(&a);
    app_hid_complete(&a, 19000);
    assert(a.state == STATE_WAIT);
    arm_and_depart(&a, 20000);
    app_poll(&a, 30000);
    app_hid_submitted(&a);
    assert(app_set_mode(&a, MODE_MEASURE, 30001) == RESULT_OK);
    assert(app_set_mode(&a, MODE_TEST, 30002) == RESULT_BUSY);
    app_hid_complete(&a, 31000);
    assert(a.state == STATE_RELEASE);
    c.action = ACTION_MOUSE;
    c.keycode = 0;
    c.mouse_buttons = 1;
    assert(app_configure(&a, &c));
    assert(app_hid_request(&a, &action, &press) && !press && action.action == ACTION_KEYBOARD);
    app_hid_submitted(&a);
    app_hid_complete(&a, 32000);
    assert(app_set_mode(&a, MODE_TEST, 33000) == RESULT_OK);
    arm_and_depart(&a, 33000);
    assert(app_set_mode(&a, MODE_MEASURE, 39000) == RESULT_OK);
    app_poll(&a, 100000);
    assert(a.state == STATE_WAIT && !app_hid_request(&a, &action, &press));
    // LOW is a legitimate clamped zero; clipped or late samples are not usable.
    c = config();
    c.reference[0] = 0;
    assert(app_configure(&a, &c));
    assert(app_set_mode(&a, MODE_TEST, 0) == RESULT_OK);
    for (unsigned i = 0; i < 3; ++i) {
        s = sample(i * 1000, 0);
        s.value[0] = 0;
        s.low_mask = 1;
        app_sample(&a, &s);
    }
    assert(a.state == STATE_ARM);
    s = sample(3000, 0);
    s.clip_mask = 1;
    app_sample(&a, &s);
    assert(!a.confirming && a.state == STATE_ARM);
    app_disconnect(&a);
    assert(a.mode == MODE_MEASURE && a.state == STATE_WAIT && !a.in_flight);
    c = config();
    assert(app_configure(&a, &c));
    uint32_t start = UINT32_MAX - 7000;
    assert(app_set_mode(&a, MODE_TEST, start) == RESULT_OK);
    arm_and_depart(&a, start);
    app_poll(&a, start + 10000);
    assert(a.state == STATE_PRESS);
    app_hid_submitted(&a);
    app_hid_failed(&a);
    assert(a.state == STATE_RELEASE && a.mode == MODE_MEASURE);
    assert(app_hid_request(&a, &action, &press) && !press);
    app_disconnect(&a);
    assert(app_set_mode(&a, MODE_TEST, 0) == RESULT_OK);
    s = sample(0, 0);
    app_sample(&a, &s);
    // The mode-entry backlog barrier must not reject samples after 35.8 minutes.
    a.confirming = false;
    arm_and_depart(&a, 0x80001000u);
}
static void test_frequency(void) {
    bool low, clipped;
    assert(sample_frequency(2500, 125000000, 15000, &low, &clipped) == 1667);
    assert(!low && !clipped);
    assert(!sample_frequency(2500, 125000000, 20000, &low, &clipped) && low);
    assert(!sample_frequency(0, 125000000, 15000, &low, &clipped) && low);
    assert(!sample_frequency(UINT32_MAX, 125000000, 15000, &low, &clipped));
    assert(sample_frequency(1, 125000000, 15000, &low, &clipped) == UINT16_MAX && clipped);
}
static void test_adaptive_sampling(void) {
    assert(sample_channel_slot_us(4000) == 1040 && sample_rate_hz(4000) == 240);
    assert(sample_channel_slot_us(10000) == 440 && sample_rate_hz(10000) == 568);
    assert(sample_channel_slot_us(15000) == 307 && sample_rate_hz(15000) == 814);
    assert(sample_channel_slot_us(20000) == 250 && sample_rate_hz(600000) == 1000);
    uint32_t previous = 0;
    for (uint32_t low = SENSOR_LOW_MIN_HZ; low <= SENSOR_LOW_MAX_HZ; ++low) {
        uint32_t rate = sample_rate_hz(low);
        assert(rate >= 240 && rate <= 1000 && rate >= previous);
        assert(sample_period_us(low) <= 1000000u / 240u);
        previous = rate;
    }
    test_config_t c = config();
    c.low_hz = SENSOR_LOW_MIN_HZ - 1;
    assert(!config_valid(&c));
    c.low_hz = SENSOR_LOW_MAX_HZ + 1;
    assert(!config_valid(&c));
    c.low_hz = 4000;
    c.arm_ms = c.depart_ms = 5;
    app_t a;
    app_init(&a);
    assert(app_configure(&a, &c));
    assert(app_set_mode(&a, MODE_TEST, 0) == RESULT_OK);
    for (unsigned i = 0; i < 3; ++i) {
        sample_t s = sample(i * 4160u, 0);
        app_sample(&a, &s);
        assert(a.state == (i == 2 ? STATE_ARM : STATE_WAIT));
    }
    for (unsigned i = 3; i < 6; ++i) {
        sample_t s = sample(i * 4160u, 30);
        app_sample(&a, &s);
    }
    assert(a.state == STATE_DELAY);
    app_set_mode(&a, MODE_TEST, 0);
    sample_t s = sample(100000, 0);
    app_sample(&a, &s);
    s = sample(108320, 0);
    app_sample(&a, &s);
    assert(a.state == STATE_WAIT && !a.confirming);
}
int main(void) {
    test_protocol();
    test_app();
    test_frequency();
    test_adaptive_sampling();
    puts("protocol, configuration, trigger/release, timer wrap and frequency checks passed");
    return 0;
}
