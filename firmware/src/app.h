#ifndef SENSOR_APP_H
#define SENSOR_APP_H
#include "protocol.h"
typedef struct {
    test_config_t config, active;
    uint8_t mode, state;
    bool configured, confirming, in_flight, cancel_press, awaiting_sample;
    uint32_t confirm_us, deadline_us, last_sample_us, test_start_us;
} app_t;
void app_init(app_t *app);
bool app_configure(app_t *app, const test_config_t *config);
uint8_t app_set_mode(app_t *app, uint8_t mode, uint32_t now_us);
void app_sample(app_t *app, const sample_t *sample);
void app_poll(app_t *app, uint32_t now_us);
bool app_hid_request(const app_t *app, test_config_t *action, bool *press);
void app_hid_submitted(app_t *app);
void app_hid_complete(app_t *app, uint32_t now_us);
void app_hid_failed(app_t *app);
void app_disconnect(app_t *app);
#endif
