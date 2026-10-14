#include "sensor.h"
#include "hardware/clocks.h"
#include "hardware/pio.h"
#include "period.pio.h"
#include "pico/stdlib.h"

queue_t sample_queue;
volatile uint32_t sensor_low_hz = 15000, capture_count, capture_drops;
static PIO sensor_pio = pio0;
static uint sensor_sm, program_offset;

void sensor_init(void) {
    queue_init(&sample_queue, sizeof(sample_t), 64);
    const uint outputs[] = {SENSOR_S2_PIN, SENSOR_S3_PIN, SENSOR_S0_PIN, SENSOR_S1_PIN, SENSOR_OE_PIN};
    for (unsigned i = 0; i < sizeof(outputs) / sizeof(outputs[0]); ++i) {
        gpio_init(outputs[i]);
        gpio_put(outputs[i], 0);
        gpio_set_dir(outputs[i], GPIO_OUT);
    }
    gpio_put(SENSOR_S0_PIN, 1);
    gpio_put(SENSOR_S1_PIN, 1);
    pio_gpio_init(sensor_pio, SENSOR_OUT_PIN);
    gpio_pull_down(SENSOR_OUT_PIN);
    sensor_sm = pio_claim_unused_sm(sensor_pio, true);
    program_offset = pio_add_program(sensor_pio, &sensor_period_program);
    pio_sm_config config = sensor_period_program_get_default_config(program_offset);
    sm_config_set_in_pins(&config, SENSOR_OUT_PIN);
    sm_config_set_jmp_pin(&config, SENSOR_OUT_PIN);
    sm_config_set_clkdiv(&config, 1.0f);
    pio_sm_init(sensor_pio, sensor_sm, program_offset, &config);
    pio_sm_set_consecutive_pindirs(sensor_pio, sensor_sm, SENSOR_OUT_PIN, 1, false);
}
void sensor_run(void) {
    // This core owns capture; USB backpressure never extends a channel's deadline.
    const uint8_t filter[] = {0, 3, 2, 1}; // R, G, B, Clear: S2 is bit 0.
    const uint32_t clock_hz = clock_get_hz(clk_sys);
    uint64_t frame_start = time_us_64() + 1000;
    uint32_t sequence = 0;
    bool previous_late = false;
    while (true) {
        busy_wait_until(from_us_since_boot(frame_start));
        sample_t sample = {.sequence = sequence++, .timestamp_us = (uint32_t)frame_start};
        uint32_t low_hz = sensor_low_hz;
        if (previous_late || time_us_64() > frame_start + 5)
            sample.flags |= SAMPLE_LATE;
        for (unsigned channel = 0; channel < 4; ++channel) {
            uint64_t start = frame_start + channel * CHANNEL_SLOT_US;
            uint64_t cutoff = start + CHANNEL_SLOT_US - 10;
            busy_wait_until(from_us_since_boot(start));
            if (time_us_64() > start + 5)
                sample.flags |= SAMPLE_LATE;
            pio_sm_set_enabled(sensor_pio, sensor_sm, false);
            pio_sm_clear_fifos(sensor_pio, sensor_sm);
            pio_sm_restart(sensor_pio, sensor_sm);
            gpio_put_masked((1u << SENSOR_S2_PIN) | (1u << SENSOR_S3_PIN),
                            ((filter[channel] & 1u) << SENSOR_S2_PIN) |
                            (((filter[channel] >> 1) & 1u) << SENSOR_S3_PIN));
            busy_wait_us_32(1);
            pio_sm_exec(sensor_pio, sensor_sm, pio_encode_jmp(program_offset));
            pio_sm_set_enabled(sensor_pio, sensor_sm, true);
            busy_wait_until(from_us_since_boot(cutoff));
            pio_sm_set_enabled(sensor_pio, sensor_sm, false);
            bool low = true, clipped = false;
            if (time_us_64() > start + CHANNEL_SLOT_US) {
                sample.flags |= SAMPLE_LATE;
            } else if (!pio_sm_is_rx_fifo_empty(sensor_pio, sensor_sm)) {
                uint32_t counts = pio_sm_get(sensor_pio, sensor_sm);
                sample.value[channel] = sample_frequency(counts, clock_hz, low_hz, &low, &clipped);
            }
            if (low)
                sample.low_mask |= (uint8_t)(1u << channel);
            if (clipped)
                sample.clip_mask |= (uint8_t)(1u << channel);
        }
        ++capture_count;
        if (!queue_try_add(&sample_queue, &sample))
            ++capture_drops;
        frame_start += 1000;
        uint64_t now = time_us_64();
        previous_late = now > frame_start;
        if (previous_late) {
            uint32_t skipped = (uint32_t)((now - frame_start) / 1000 + 1);
            sequence += skipped;
            capture_drops += skipped;
            frame_start += (uint64_t)skipped * 1000;
        }
    }
}
