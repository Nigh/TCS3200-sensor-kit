---
name: TCS3200 测量台
description: Xianii dark native Linux measurement workspace
colors:
  background: "#161616"
  surface: "#242424"
  border: "#404040"
  text: "#f2f2f3"
  muted: "#b4b4b7"
  primary: "#ff9db1"
  primary-hover: "#ffb4c2"
  channel-red: "#ff9db1"
  channel-green: "#8bd5a3"
  channel-blue: "#8bbdf0"
  channel-clear: "#e1e4ed"
  grid-horizontal: "#343434"
  grid-vertical: "#303030"
typography:
  title:
    fontFamily: "Noto Sans CJK SC"
    fontSize: "22px"
    fontWeight: 700
  reading:
    fontFamily: "Noto Sans CJK SC"
    fontSize: "20px"
  body:
    fontFamily: "Noto Sans CJK SC"
    fontSize: "14px"
  message:
    fontFamily: "Noto Sans CJK SC"
    fontSize: "13px"
  label:
    fontFamily: "Noto Sans CJK SC"
    fontSize: "12px"
rounded:
  control: "8px"
  panel: "8px"
spacing:
  axis-gap: "6px"
  group: "8px"
  section: "12px"
  panel-inset: "14px"
  heading-gap: "16px"
  workspace-inset: "20px"
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.surface}"
    rounded: "{rounded.control}"
  button-primary-hover:
    backgroundColor: "{colors.primary-hover}"
  measurement-panel:
    backgroundColor: "{colors.surface}"
    rounded: "{rounded.panel}"
    padding: "{spacing.panel-inset}"
  statistics-strip:
    textColor: "{colors.muted}"
    typography: "{typography.label}"
---

# Design System: TCS3200 测量台

## Overview

**Creative North Star: "Measurement workspace"**

Operate mode: a compact, Chinese-language native Linux instrument. The confirmed visual authority is [xianii-theme](https://github.com/Nigh/xianii-theme), translated into MyGo rather than HTML/CSS. Neutral dark surfaces and pink actions keep the measurement traces central.

This document reconciles the initial design draft with `main.go` and `PRODUCT.md`. It records the shipped surface, not a new aesthetic direction. Acquisition continues independently while the operator examines a frozen snapshot; visible transport quality and persistent messages belong alongside the measurements.

**Key Characteristics:**
- Compact native controls and restrained tonal layering.
- Four equal channel plots with linked time axes and cursor.
- Stable statistics strips and explicit units.
- Persistent acquisition quality and recovery information.

## Colors

The palette pairs charcoal neutrals with a soft pink action accent and four named channel colors. Frontmatter contains the exact implemented sRGB values.

### Primary
- **Xianii Pink** (`primary`): connection action, connection state, live/snapshot state, native focus, and the red trace.
- **Hover Pink** (`primary-hover`): primary control hover state.

### Secondary
- **Channel Green**, **Channel Blue**, **Channel Clear**: green, blue and near-white clear traces and matching labels. Channel Red shares the pink accent.

### Neutral
- **Workspace Charcoal** (`background`): application background.
- **Panel Charcoal** (`surface`): plot containers and primary-action text.
- **Control Border** (`border`): native control boundaries.
- **Reading White** (`text`): main labels and numeric readings.
- **Metadata Gray** (`muted`): help, statistics, quality footer and linked cursor.
- **Horizontal Grid** and **Vertical Grid**: subdued plot structure below the traces.

Warning flags and error messages use inherited MyGo `Warning` and `Danger` roles. Their values are not app-defined tokens here.

**The Channel Identity Rule.** Always pair channel color with its textual channel name; color alone does not identify a measurement.

## Typography

**UI Font:** Noto Sans CJK SC, with MyGo's native fallback. The app specifies no separate display or monospace family, custom line height or tracking.

The hierarchy favors legible Chinese controls and consistently formatted numerical readings. Token sizes are portable px notation for native logical UI units (DIP), not a CSS implementation contract.

- **Title:** bold application name.
- **Reading:** channel pulse width with one decimal place and μs units.
- **Body:** controls and channel labels; channel names are bold.
- **Message:** persistent transport or validation message.
- **Label:** help, metadata, warning flags and hover statistics.

## Layout

The window opens at 1280 × 900 with a 1080 × 760 minimum. A flexible main area sits beside a 288 DIP control rail. Acquisition settings and channel switches sit at the top right. Device enumeration, selection, connect/disconnect, state, transport messages and HID status sit at the bottom right. Quality counters remain in the footer.

Enabled waveforms stack vertically and share the available height equally. Disabled channels occupy no space; all-disabled acquisition shows an explicit empty state. Each row pairs an independent 176 DIP bordered readout card with a growing chart. Colored R/G/B/C badges precede a separate Chinese channel name, pulse width, minimum/maximum and flags. Matching mappings add a vertical response bar inside the readout card. No heading or statistics strip reduces chart height.

The device selector is 264 DIP wide inside a 12 DIP panel inset. Desktop window resizing expands the charts rather than the control rail. There are no mobile breakpoints.

## Elevation & Depth

App-defined depth comes from tonal layering: charcoal panels sit against the darker workspace, and subdued grids sit beneath the colored traces. There are no app-defined shadows, decorative gradients, raster assets or ornamental animations. Standard control interaction treatments remain owned by MyGo.

## Shapes

Gently rounded controls and measurement panels share the radius tokens. Plot geometry is clipped to its rectangular drawing area. Traces use a 1.5-logical-unit stroke; grid and linked cursor lines use a 1-logical-unit stroke. The chart is the dominant geometry rather than a decorative card treatment.

## Components

### Native controls

Use MyGo `Button`, `PrimaryButton`, `Select` and `TextInput`. Connection is the pink primary action; enumeration, disconnect, apply, pause and return-to-live use standard buttons. Preserve framework hover, keyboard focus and disabled states rather than inventing parallel styling. The selector and threshold field retain their accessible labels. Threshold edits normalize on blur or apply, within 100–600000 Hz; rate preview is distinguished from the device-applied maximum. Busy, disconnected and invalid-selection states disable the relevant controls.

### Measurement panel

Each horizontal measurement row pairs a readout column with a growing chart. Empty plots show “等待 USB 采样” and the connection path. Flags use the framework warning role. Clear uses near-white to match the reference layout.

### Linked inspection

All enabled panels share the time range and cursor fraction. Time labels and hover times start at zero on the viewport's left edge; the default ten-second view spans 0–10 s. Pointer-anchored zoom and drag pan freeze the snapshot while acquisition continues. One pause/resume button and Escape restore live display; there is no duplicate return-to-live button. Native 数值波形 / 频谱 tabs share the inspection toolbar and preserve snapshots when switching.

### Hover statistics

Only the hovered chart shows a pass-through native overlay bubble near the pointer or raw sample. `AttachTo` keeps the bubble inside the window. Show bucket bounds, sample count, maximum, minimum and median, or raw sample pulse width and time. LOW explanations appear in the bubble and chart accessibility description. Moving away removes the bubble; inspection never changes chart geometry.

### Persistent metadata

The bottom-right device panel retains transport/measurement messages and HID polling status. The footer retains drop/decode counters, protocol errors and threshold. Error state uses the framework danger role. Synthetic demonstration data remains explicitly labeled.

Native components have no shipped HTML/CSS snippets. The sidecar deliberately omits browser component previews and mobile breakpoints rather than fabricating an alternate implementation.

When each occupied physical pixel column holds at most one enabled-channel sample, show individual pulse width and time relative to the viewport's left edge, formatted to 0.1 ms, in the hover bubble. Draw 3 DIP radius hollow markers with a 1.5 DIP channel-colored outline; fill the marker within 6 DIP of the pointer. Snap the cursor to the nearest actual sample. Dense views show raw-data bucket statistics. MyGo v0.10.0 supplies `chart.MinMax` and `Painter.StrokePolyline`; the app reuses drawing buffers.

## Do's and Don'ts

### Do:
- **Do** preserve the confirmed Xianii dark palette and native Chinese interface.
- **Do** keep the four channel time axes and inspection cursor linked.
- **Do** retain explicit channel names, units, quality counters and persistent errors.
- **Do** let charts fill each row and preserve native focus and disabled states.

### Don't:
- **Don't** identify channels by color alone.
- **Don't** cover measurements with statistics tooltips or move plot geometry during inspection.
- **Don't** present synthetic demonstration data as sensor measurements.
- **Don't** introduce a web styling layer or ornamental animation into this native surface.

### Workspace tabs

The three workspace tabs use native `TabsBase` semantics with equal flex widths and a 36 DIP tab height. The strip has a surface background, 1 DIP border, 8 DIP radius and 4 DIP inset/gap. The selected tab has an accent fill and accent-text label; the inactive tab uses muted text and a border-color hover fill. Each tab has a 5 DIP radius. Keep native tab roles, keyboard selection and focus indicators.


### Screen color mapping and mapped readings

The third tab preserves the native dark workspace. A 360 DIP unadorned square displays calibration colors; controls, four pulse readings, progress, failure and reset confirmation remain outside it. Disable tab and acquisition changes during a run. Use the existing native buttons and keyboard focus semantics.

The third tab is named 屏幕颜色映射. With a matching map, show a bottom-up vertical channel-colored response bar in the left readout card, with percentage and channel/value text. The inspection row retains its color swatch and keyboard-operable RGB/HEX/HSL button. Charts use a 48 DIP minimum allocation including axes.

### Spectrum

The frequency mode reuses channel cards and flex heights. Each card shows its actual FFT point count and Δf instead of waveform extrema. Analyze only the latest valid contiguous suffix inside the current viewport, using 32–4096 power-of-two points, timestamp-derived rate, DC removal and a Hann window. Use a linear single-sided amplitude scale in μs and 0–Nyquist frequency ticks in Hz/kHz. Annotate each channel's dominant prominent peak with a pass-through bubble attached to the peak, without hover or reserved vertical space. Flat/noisy signals have no peak label; missing or irregular data shows an explicit waiting/error state. Cache FFT results and reuse transform/drawing buffers.
