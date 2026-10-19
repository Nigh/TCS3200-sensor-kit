# Product

<!-- impeccable:product-schema 1 -->

## Platform
Linux desktop, native Go UI (not a web surface).

## Stack
MyGo native UI and chart primitives; USB CDC through go.bug.st/serial. No web frontend.

## Users
Sensor-kit operators measuring screen light on the current Linux desktop.

## Product Purpose
Observe four TCS3200 pulse-width channels at a threshold-dependent maximum cadence of 240–1000 Hz, inspect short time ranges and pixel-bucket statistics, and adjust the physical measurement low-frequency threshold.

## Capabilities and Constraints
Manual device enumeration, selection and connection; latest ten seconds only; frozen snapshot while USB reception continues; robust disconnect handling. Low threshold is clamped to 4000–600000 Hz on blur/submit, and applied/draft maximum sample rates are displayed. Firmware and host require matching v2; nominal cadence is floor(1000000/(4*max(250,ceil(4000000/low_hz)+40))) Hz. Protocol v2 in ../docs/protocol.md remains authoritative. No test-mode controls, HID actions, BOOTSEL, export or longer history in this version.

## Brand Commitments
Chinese UI. Xianii dark theme, translated into native UI tokens. Direct implementation without generated mockups, as selected by the user.

## Evidence on Hand
Firmware, protocol and wire test vectors. Hardware was not present during initial planning; synthetic test signals must be labeled.

## Product Principles
- Keep acquisition independent of rendering.
- Preserve raw samples for statistics; reduce drawing geometry only.
- Bound memory and never silently reconnect or retry state-changing commands.
- Show transport and measurement quality alongside waveforms.
