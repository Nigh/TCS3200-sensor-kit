#include "usb_func.h"
#include "hardware/regs/usb.h"
#include "hardware/structs/usb.h"
#include "hardware/structs/usb_dpram.h"
#include "pico/stdlib.h"
#include "hardware/sync.h"

static uint32_t window_start, last_task, polls;
static uint16_t rate = UINT16_MAX;
static bool measuring;

void usb_poll_reset(void) {
    measuring = false;
    rate = UINT16_MAX;
    polls = 0;
}
void usb_poll_task(bool idle) {
    uint32_t now = time_us_32();
    // Observe EP1 IN NAKs without enabling another USB interrupt or sending reports.
    uint32_t irq = save_and_disable_interrupts();
    bool enabled = (usb_dpram->ep_ctrl[0].in & EP_CTRL_ENABLE_BITS) != 0;
    if (enabled)
        usb_dpram->ep_ctrl[0].in |= EP_CTRL_INTERRUPT_ON_NAK;
    restore_interrupts(irq);
    if (!enabled) {
        usb_poll_reset();
        return;
    }
    bool nak = (usb_hw->ep_nak_stall_status & USB_EP_STATUS_STALL_NAK_EP1_IN_BITS) != 0;
    usb_hw->ep_nak_stall_status = USB_EP_STATUS_STALL_NAK_EP1_IN_BITS;
    // shortcut: NAK flags coalesce; discard windows with >=1ms observer gaps, use a USB analyzer for bus-level timing.
    if (!idle || (measuring && (uint32_t)(now - last_task) >= HID_INTERVAL_MS * 1000u))
        usb_poll_reset();
    last_task = now;
    if (!idle)
        return;
    if (!measuring) {
        measuring = true;
        window_start = now;
        polls = 0;
        return;
    }
    if (nak)
        ++polls;
    uint32_t elapsed = now - window_start;
    if (elapsed >= 1000000u) {
        rate = (uint16_t)((uint64_t)polls * 1000000u / elapsed);
        window_start = now;
        polls = 0;
    }
}
uint16_t usb_poll_rate(void) {
    return rate;
}
