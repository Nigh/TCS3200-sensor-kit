#include <stdio.h>
#include "pico/stdlib.h"
#include "hardware/clocks.h"
#include "hardware/dma.h"
#include "hardware/pio.h"
#include "sensor.h"
#include "step_response.pio.h"

#define LED_PIN 28u
#define WORDS 31250u
static uint32_t samples[WORDS];

static void output(uint pin, bool value) {
    gpio_init(pin);
    gpio_put(pin, value);
    gpio_set_dir(pin, GPIO_OUT);
}

static bool capture(PIO pio, uint sm, uint offset, int dma, bool rise, bool fast) {
    pio_sm_set_enabled(pio, sm, false);
    pio_sm_clear_fifos(pio, sm);
    pio_sm_restart(pio, sm);
    pio->fdebug = 1u << (PIO_FDEBUG_RXSTALL_LSB + sm);
    gpio_set_function(LED_PIN, GPIO_FUNC_SIO);
    gpio_put(LED_PIN, rise);
    sleep_us(20000u + (time_us_32() & 1023u));

    pio_sm_config config = step_response_program_get_default_config(offset);
    sm_config_set_in_pins(&config, SENSOR_OUT_PIN);
    sm_config_set_sideset_pins(&config, LED_PIN);
    sm_config_set_in_shift(&config, true, true, 32);
    sm_config_set_clkdiv(&config, fast ? 12.5f : 250.0f);
    pio_sm_init(pio, sm, offset + (rise ? step_response_offset_rise : step_response_offset_fall), &config);
    pio_sm_set_pins_with_mask(pio, sm, rise ? (1u << LED_PIN) : 0, 1u << LED_PIN);
    pio_sm_set_pindirs_with_mask(pio, sm, 1u << LED_PIN, 1u << LED_PIN);
    pio_sm_put_blocking(pio, sm, 499999);
    pio_sm_put_blocking(pio, sm, 499999);
    gpio_put(LED_PIN, true);
    pio_gpio_init(pio, LED_PIN);

    dma_channel_config dc = dma_channel_get_default_config(dma);
    channel_config_set_transfer_data_size(&dc, DMA_SIZE_32);
    channel_config_set_read_increment(&dc, false);
    channel_config_set_write_increment(&dc, true);
    channel_config_set_dreq(&dc, pio_get_dreq(pio, sm, false));
    dma_hw->ch[dma].ctrl_trig = DMA_CH0_CTRL_TRIG_READ_ERROR_BITS | DMA_CH0_CTRL_TRIG_WRITE_ERROR_BITS;
    dma_channel_configure(dma, &dc, samples, &pio->rxf[sm], WORDS, true);
    pio_sm_set_enabled(pio, sm, true);
    absolute_time_t deadline = make_timeout_time_ms(fast ? 1200 : 5000);
    while (dma_channel_is_busy(dma) && !time_reached(deadline)) tight_loop_contents();
    bool timeout = dma_channel_is_busy(dma);
    gpio_set_function(LED_PIN, GPIO_FUNC_SIO);
    pio_sm_set_enabled(pio, sm, false);
    if (timeout) dma_channel_abort(dma);
    uint32_t errors = dma_hw->ch[dma].ctrl_trig & (DMA_CH0_CTRL_TRIG_READ_ERROR_BITS | DMA_CH0_CTRL_TRIG_WRITE_ERROR_BITS);
    bool stalled = (pio->fdebug & (1u << (PIO_FDEBUG_RXSTALL_LSB + sm))) != 0;
    return !timeout && !errors && !stalled;
}

int main(void) {
    set_sys_clock_khz(125000, true);
    output(LED_PIN, true);
    output(SENSOR_S0_PIN, true);
    output(SENSOR_S1_PIN, true);
    output(SENSOR_S2_PIN, true);
    output(SENSOR_S3_PIN, false);
    output(SENSOR_OE_PIN, false);
    gpio_init(SENSOR_OUT_PIN);
    gpio_set_dir(SENSOR_OUT_PIN, GPIO_IN);
    gpio_pull_down(SENSOR_OUT_PIN);
    stdio_init_all();
    PIO pio = pio0;
    uint sm = pio_claim_unused_sm(pio, true);
    uint offset = pio_add_program(pio, &step_response_program);
    int dma = dma_claim_unused_channel(true);
    while (true) {
        int c = getchar_timeout_us(10000);
        if (c != 'R' && c != 'F' && c != 'r' && c != 'f') continue;
        bool rise = c == 'R' || c == 'r';
        bool fast = c == 'R' || c == 'F';
        if (!capture(pio, sm, offset, dma, rise, fast)) {
            stdio_put_string("ERR capture\n", 12, false, false);
            continue;
        }
        char header[64];
        int n = snprintf(header, sizeof(header), "STEP1 %c %u 1000000 500000\n", c, fast ? 5000000u : 250000u);
        stdio_put_string(header, n, false, false);
        stdio_put_string((const char *)samples, sizeof(samples), false, false);
    }
}
