#include "app.h"
#include <string.h>

static void cancel(app_t *a) {
    a->confirming = false;
    if (a->state == STATE_HOLD)
        a->state = STATE_RELEASE;
    else if (a->state == STATE_PRESS && a->in_flight)
        a->cancel_press = true;
    else if (a->state != STATE_RELEASE)
        a->state = STATE_WAIT;
}
void app_init(app_t *a) {
    memset(a, 0, sizeof(*a));
    a->config.low_hz = 15000;
}
bool app_configure(app_t *a, const test_config_t *config) {
    if (!config_valid(config))
        return false;
    cancel(a);
    a->mode = MODE_MEASURE;
    a->config = *config;
    a->configured = true;
    return true;
}
uint8_t app_set_mode(app_t *a, uint8_t mode, uint32_t now_us) {
    if (mode > MODE_TEST)
        return RESULT_VALUE;
    if (mode == MODE_TEST && !a->configured)
        return RESULT_NOT_CONFIGURED;
    if (mode == MODE_TEST && a->state >= STATE_PRESS)
        return RESULT_BUSY;
    cancel(a);
    a->mode = mode;
    a->test_start_us = now_us;
    a->awaiting_sample = mode == MODE_TEST;
    return RESULT_OK;
}
void app_sample(app_t *a, const sample_t *s) {
    if (a->mode != MODE_TEST || (a->state != STATE_WAIT && a->state != STATE_ARM))
        return;
    if (a->awaiting_sample) {
        if ((int32_t)(s->timestamp_us - a->test_start_us) < 0)
            return;
        a->awaiting_sample = false;
    }
    if ((s->clip_mask & 7u) || s->flags ||
        (a->confirming && (uint32_t)(s->timestamp_us - a->last_sample_us) != 1000u)) {
        a->confirming = false;
        return;
    }
    a->last_sample_us = s->timestamp_us;
    // shortcut: LOW is zero; add a presence input if black must differ from disconnection.
    uint16_t distance = 0;
    for (unsigned i = 0; i < 3; ++i) {
        int difference = (int)s->value[i] - a->config.reference[i];
        uint16_t absolute = (uint16_t)(difference < 0 ? -difference : difference);
        if (absolute > distance)
            distance = absolute;
    }
    bool qualifies = a->state == STATE_WAIT ? distance <= a->config.enter_tolerance
                                            : distance >= a->config.exit_threshold;
    if (!qualifies) {
        a->confirming = false;
        return;
    }
    if (!a->confirming) {
        a->confirming = true;
        a->confirm_us = s->timestamp_us;
    }
    uint32_t duration = (a->state == STATE_WAIT ? a->config.arm_ms : a->config.depart_ms) * 1000u;
    if ((uint32_t)(s->timestamp_us - a->confirm_us) < duration)
        return;
    a->confirming = false;
    if (a->state == STATE_WAIT) {
        a->state = STATE_ARM;
    } else {
        a->active = a->config;
        a->state = STATE_DELAY;
        a->deadline_us = s->timestamp_us + a->active.delay_ms * 1000u;
    }
}
void app_poll(app_t *a, uint32_t now_us) {
    if ((a->state == STATE_DELAY || a->state == STATE_HOLD) &&
        (int32_t)(now_us - a->deadline_us) >= 0)
        a->state = a->state == STATE_DELAY ? STATE_PRESS : STATE_RELEASE;
}
bool app_hid_request(const app_t *a, test_config_t *action, bool *press) {
    if (a->in_flight || (a->state != STATE_PRESS && a->state != STATE_RELEASE))
        return false;
    *action = a->active;
    *press = a->state == STATE_PRESS;
    return true;
}
void app_hid_submitted(app_t *a) {
    a->in_flight = true;
}
void app_hid_complete(app_t *a, uint32_t now_us) {
    if (!a->in_flight)
        return;
    a->in_flight = false;
    if (a->state == STATE_PRESS) {
        a->state = a->cancel_press ? STATE_RELEASE : STATE_HOLD;
        a->deadline_us = now_us + a->active.hold_ms * 1000u;
        a->cancel_press = false;
    } else if (a->state == STATE_RELEASE) {
        a->state = STATE_WAIT;
    }
}
void app_hid_failed(app_t *a) {
    a->in_flight = false;
    // A failed press may have reached the host; always follow it with a release.
    a->state = STATE_RELEASE;
    a->cancel_press = false;
    a->mode = MODE_MEASURE;
}
void app_disconnect(app_t *a) {
    a->mode = MODE_MEASURE;
    a->state = STATE_WAIT;
    a->in_flight = false;
    a->cancel_press = false;
    a->confirming = false;
}
