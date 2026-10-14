#ifndef SENSOR_PROTOCOL_H
#define SENSOR_PROTOCOL_H
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#define PROTOCOL_VERSION 1u
#define PROTOCOL_MAX_PAYLOAD 64u
#define PROTOCOL_OVERHEAD 10u
#define PROTOCOL_MAX_FRAME (PROTOCOL_MAX_PAYLOAD + PROTOCOL_OVERHEAD)
#define CONFIG_SIZE 28u
#define SAMPLE_SIZE 22u

enum {
    CMD_STATUS = 0x01, CMD_MODE = 0x02, CMD_CONFIG = 0x03,
    CMD_GET_CONFIG = 0x04, CMD_BOOT = 0x05, CMD_LOW = 0x06,
    MSG_SAMPLE = 0x40, MSG_ACK = 0x80, MSG_STATUS = 0x81, MSG_CONFIG = 0x84
};
enum { RESULT_OK, RESULT_LENGTH, RESULT_VALUE, RESULT_NOT_CONFIGURED, RESULT_BUSY, RESULT_UNKNOWN };
enum { MODE_MEASURE, MODE_TEST };
enum { STATE_WAIT, STATE_ARM, STATE_DELAY, STATE_PRESS, STATE_HOLD, STATE_RELEASE };
enum { ACTION_KEYBOARD = 1, ACTION_MOUSE = 2 };
enum { SAMPLE_LATE = 1 };

typedef struct {
    uint32_t low_hz;
    uint16_t reference[3];
    uint16_t enter_tolerance, exit_threshold, arm_ms, depart_ms;
    uint32_t delay_ms;
    uint16_t hold_ms;
    uint8_t action, modifiers, keycode, mouse_buttons;
} test_config_t;
typedef struct {
    uint32_t sequence, timestamp_us;
    uint16_t value[4];
    uint8_t low_mask, clip_mask, flags;
} sample_t;
typedef void (*protocol_handler_t)(uint8_t type, uint16_t sequence,
                                   const uint8_t *payload, uint16_t length, void *context);
typedef struct {
    uint8_t data[PROTOCOL_MAX_FRAME];
    size_t used;
    uint32_t errors, last_byte_us;
} protocol_parser_t;

uint16_t wire_u16(const uint8_t *p);
uint32_t wire_u32(const uint8_t *p);
void wire_put16(uint8_t *p, uint16_t value);
void wire_put32(uint8_t *p, uint32_t value);
uint16_t protocol_crc(const uint8_t *data, size_t length);
size_t protocol_encode(uint8_t *out, uint8_t type, uint16_t sequence,
                       const uint8_t *payload, uint16_t length);
void protocol_feed(protocol_parser_t *parser, const uint8_t *data, size_t length,
                   uint32_t now_us, protocol_handler_t handler, void *context);
void config_decode(test_config_t *config, const uint8_t data[CONFIG_SIZE]);
void config_encode(uint8_t data[CONFIG_SIZE], const test_config_t *config);
bool config_valid(const test_config_t *config);
uint16_t sample_frequency(uint32_t counts, uint32_t clock_hz, uint32_t low_hz,
                          bool *low, bool *clipped);
#endif
