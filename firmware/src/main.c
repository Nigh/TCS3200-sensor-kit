#include "pico/multicore.h"
#include "pico/stdlib.h"
#include "sensor.h"
#include "usb_func.h"

int main(void) {
    sensor_init();
    usb_init();
    multicore_launch_core1(sensor_run);
    while (true) {
        usb_task();
        sample_t sample;
        // Bound each pass so a backlog cannot starve TinyUSB and HID releases.
        for (unsigned i = 0; i < 8 && queue_try_remove(&sample_queue, &sample); ++i)
            usb_sample(&sample);
        tight_loop_contents();
    }
}
