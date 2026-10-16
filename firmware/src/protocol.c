#include "protocol.h"
#include <string.h>

uint16_t wire_u16(const uint8_t *p) {
    return (uint16_t)(p[0] | (uint16_t)p[1] << 8);
}
uint32_t wire_u32(const uint8_t *p) {
    return (uint32_t)wire_u16(p) | (uint32_t)wire_u16(p + 2) << 16;
}
void wire_put16(uint8_t *p, uint16_t value) {
    p[0] = (uint8_t)value;
    p[1] = (uint8_t)(value >> 8);
}
void wire_put32(uint8_t *p, uint32_t value) {
    wire_put16(p, (uint16_t)value);
    wire_put16(p + 2, (uint16_t)(value >> 16));
}
uint16_t protocol_crc(const uint8_t *data, size_t length) {
    uint16_t crc = 0xffff;
    for (size_t i = 0; i < length; ++i) {
        crc ^= (uint16_t)data[i] << 8;
        for (unsigned bit = 0; bit < 8; ++bit)
            crc = (uint16_t)((crc << 1) ^ ((crc & 0x8000) ? 0x1021 : 0));
    }
    return crc;
}
size_t protocol_encode(uint8_t *out, uint8_t type, uint16_t sequence,
                       const uint8_t *payload, uint16_t length) {
    if (length > PROTOCOL_MAX_PAYLOAD || (length && !payload))
        return 0;
    out[0] = 0x54;
    out[1] = 0x43;
    out[2] = PROTOCOL_VERSION;
    out[3] = type;
    wire_put16(out + 4, sequence);
    wire_put16(out + 6, length);
    if (length)
        memcpy(out + 8, payload, length);
    wire_put16(out + 8 + length, protocol_crc(out + 2, length + 6));
    return length + PROTOCOL_OVERHEAD;
}
static void discard(protocol_parser_t *p, size_t count) {
    p->used -= count;
    memmove(p->data, p->data + count, p->used);
}
void protocol_feed(protocol_parser_t *p, const uint8_t *data, size_t length,
                   uint32_t now_us, protocol_handler_t handler, void *context) {
    if (p->used && (uint32_t)(now_us - p->last_byte_us) >= 100000u) {
        p->used = 0;
        ++p->errors;
    }
    if (length)
        p->last_byte_us = now_us;
    for (size_t i = 0; i < length; ++i) {
        p->data[p->used++] = data[i];
        while (p->used) {
            if (p->data[0] != 0x54 || (p->used >= 2 && p->data[1] != 0x43)) {
                discard(p, 1);
                continue;
            }
            if (p->used < 8)
                break;
            uint16_t payload_length = wire_u16(p->data + 6);
            if (p->data[2] != PROTOCOL_VERSION || payload_length > PROTOCOL_MAX_PAYLOAD) {
                ++p->errors;
                discard(p, 1);
                continue;
            }
            size_t frame_length = payload_length + PROTOCOL_OVERHEAD;
            if (p->used < frame_length)
                break;
            if (wire_u16(p->data + 8 + payload_length) != protocol_crc(p->data + 2, payload_length + 6)) {
                ++p->errors;
                discard(p, 1);
                continue;
            }
            handler(p->data[3], wire_u16(p->data + 4), p->data + 8, payload_length, context);
            discard(p, frame_length);
        }
    }
}
void config_decode(test_config_t *c, const uint8_t p[CONFIG_SIZE]) {
    c->low_hz = wire_u32(p);
    for (unsigned i = 0; i < 3; ++i)
        c->reference[i] = wire_u16(p + 4 + 2 * i);
    c->enter_tolerance = wire_u16(p + 10);
    c->exit_threshold = wire_u16(p + 12);
    c->arm_ms = wire_u16(p + 14);
    c->depart_ms = wire_u16(p + 16);
    c->delay_ms = wire_u32(p + 18);
    c->hold_ms = wire_u16(p + 22);
    c->action = p[24];
    c->modifiers = p[25];
    c->keycode = p[26];
    c->mouse_buttons = p[27];
    c->channels = p[28];
    c->channel = p[29];
}
void config_encode(uint8_t p[CONFIG_SIZE], const test_config_t *c) {
    wire_put32(p, c->low_hz);
    for (unsigned i = 0; i < 3; ++i)
        wire_put16(p + 4 + 2 * i, c->reference[i]);
    wire_put16(p + 10, c->enter_tolerance);
    wire_put16(p + 12, c->exit_threshold);
    wire_put16(p + 14, c->arm_ms);
    wire_put16(p + 16, c->depart_ms);
    wire_put32(p + 18, c->delay_ms);
    wire_put16(p + 22, c->hold_ms);
    p[24] = c->action;
    p[25] = c->modifiers;
    p[26] = c->keycode;
    p[27] = c->mouse_buttons;
    p[28] = c->channels;
    p[29] = c->channel;
}
bool config_valid(const test_config_t *c) {
    if (!c->channels || c->channels > 15 || c->channel > 3 || !(c->channels & (1u << c->channel)))
        return false;
    if (c->low_hz < SENSOR_LOW_MIN_HZ || c->low_hz > SENSOR_LOW_MAX_HZ ||
        c->exit_threshold <= c->enter_tolerance || c->arm_ms > 60000 ||
        c->depart_ms > 60000 || c->delay_ms > 1000 ||
        !c->hold_ms || c->hold_ms > 60000)
        return false;
    for (unsigned i = 0; i < 3; ++i)
        if (c->reference[i] == UINT16_MAX)
            return false;
    if (c->action == ACTION_KEYBOARD)
        return c->keycode >= 4 && c->keycode <= 0xdf && !c->mouse_buttons;
    if (c->action == ACTION_MOUSE)
        return c->mouse_buttons && c->mouse_buttons <= 31 && !c->keycode && !c->modifiers;
    return false;
}
uint32_t sample_channel_slot_us(uint32_t low_hz) {
    if (low_hz < SENSOR_LOW_MIN_HZ)
        low_hz = SENSOR_LOW_MIN_HZ;
    uint32_t slot = (SENSOR_PERIODS_PER_SLOT * 1000000u + low_hz - 1u) / low_hz + SENSOR_SLOT_OVERHEAD_US;
    return slot < SENSOR_MIN_SLOT_US ? SENSOR_MIN_SLOT_US : slot;
}
uint32_t sample_period_us(uint32_t low_hz) {
    return 4u * sample_channel_slot_us(low_hz);
}
uint16_t sample_rate_hz(uint32_t low_hz) {
    return (uint16_t)(1000000u / sample_period_us(low_hz));
}
uint16_t sample_frequency(uint32_t counts, uint32_t clock_hz, uint32_t low_hz,
                          bool *low, bool *clipped) {
    *clipped = false;
    *low = true;
    if (!counts || counts > UINT32_MAX / 3u)
        return 0;
    uint32_t cycles = counts * 3u - 1u;
    uint32_t hz = clock_hz / cycles;
    if (hz < low_hz)
        return 0;
    *low = false;
    uint32_t value = (hz + 5u) / 10u;
    *clipped = value > UINT16_MAX;
    return *clipped ? UINT16_MAX : (uint16_t)value;
}

uint32_t sample_mask_period_us(uint32_t low_hz, uint8_t channels) {
    unsigned count = 0;
    for (unsigned i = 0; i < 4; ++i) count += (channels >> i) & 1u;
    return count ? count * sample_channel_slot_us(low_hz) : 10000u;
}
uint16_t sample_mask_rate_hz(uint32_t low_hz, uint8_t channels) {
    return channels ? (uint16_t)(1000000u / sample_mask_period_us(low_hz, channels)) : 0;
}
