# Product

<!-- impeccable:product-schema 1 -->

## Platform
Linux desktop, native Go UI (not a web surface).

## Stack
MyGo native UI and chart primitives; USB CDC through go.bug.st/serial. No web frontend.

## Users
Sensor-kit operators measuring screen light on the current Linux desktop.

## Product Purpose
Measure four TCS3200 pulse-width channels, calibrate sensor responses against screen RGB, and run explicitly initiated single-channel HID trigger tests.

## Capabilities and Constraints
Three native tabs: waveforms, automatic trigger testing, screen color calibration. Manual connection; bounded ten-second history; independent acquisition and frozen snapshots. Threshold 100–600000 Hz; default 15000 Hz. Protocol v4 uses u32 frequencies in Hz and matching firmware. Slot is max(250,ceil(4000000/low_hz)+40) microseconds; four channels at 100 Hz take 160160 microseconds per group. Screen calibration scans 125 RGB colors, stores four channel pulse-width/frequency responses and quality, and uses tetrahedral interpolation. Calibrated channel bars report relative black/white response; RGB-enabled color preview reports reconstructed screen RGB. Config and calibration persist locally, with explicit reset controls; no automatic HID test start, no device flash storage or export.

## Brand Commitments
Chinese UI. Xianii dark theme, native MyGo controls and keyboard navigation.

## Evidence on Hand
Protocol golden vectors, firmware C/USB tests, PIO simulation, host acquisition/calibration/persistence tests and native UI screenshots. Hardware validation remains required for low-light limits and color accuracy.

## Product Principles
- Keep acquisition independent of rendering.
- Preserve raw samples for statistics; reduce drawing geometry only.
- Bound memory and never silently reconnect or retry state-changing commands.
- Show transport and measurement quality alongside waveforms.
- Never interpret LOW or calibration-range failures as a precise color.
- Preserve saved calibration until explicit reset or successful replacement.
