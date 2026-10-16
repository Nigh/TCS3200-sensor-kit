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
  channel-clear: "#d3b1f4"
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
- **Channel Green**, **Channel Blue**, **Channel Lilac**: green, blue and clear traces and their matching channel labels. Channel Red intentionally shares the pink accent.

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
- **Reading:** channel frequency with two decimal places and kHz units.
- **Body:** controls and channel labels; channel names are bold.
- **Message:** persistent transport or validation message.
- **Label:** help, metadata, warning flags and two-line bucket statistics.

## Layout

The shipped window opens at 1280 × 900 and has a 1080 × 760 minimum. The outer column uses the workspace inset and section gap tokens. A title/state row precedes the USB/threshold toolbar, with applied and draft maximum sampling rates shown in the heading, then a compact inspection row. A growing, equal-fraction 2 × 2 plot grid fills the remaining space, followed by persistent message and quality rows.

The plot grid has a native logical minimum height of 400; each chart has a minimum height of 120. Panel contents use the panel inset and group gap. Chart axes use the axis gap. A statistics strip reserves at least 36 logical units beneath each chart so inspection does not cover the waveform or shift its geometry. The device selector is 330 logical units wide and the threshold field is 100.

This is a desktop native layout with a supported minimum window size; there are no shipped mobile breakpoints or web container rules. Equal panel widths preserve comparable time geometry as the window grows.

## Elevation & Depth

App-defined depth comes from tonal layering: charcoal panels sit against the darker workspace, and subdued grids sit beneath the colored traces. There are no app-defined shadows, decorative gradients, raster assets or ornamental animations. Standard control interaction treatments remain owned by MyGo.

## Shapes

Gently rounded controls and measurement panels share the radius tokens. Plot geometry is clipped to its rectangular drawing area. Traces use a 1.5-logical-unit stroke; grid and linked cursor lines use a 1-logical-unit stroke. The chart is the dominant geometry rather than a decorative card treatment.

## Components

### Native controls

Use MyGo `Button`, `PrimaryButton`, `Select` and `TextInput`. Connection is the pink primary action; enumeration, disconnect, apply, pause and return-to-live use standard buttons. Preserve framework hover, keyboard focus and disabled states rather than inventing parallel styling. The selector and threshold field retain their accessible labels. Threshold edits normalize on blur or apply, within 4000–600000 Hz; rate preview is distinguished from the device-applied maximum. Busy, disconnected and invalid-selection states disable the relevant controls.

### Measurement panel

A surface container holds a channel label, latest frequency and textual LOW/CLIP/LATE flags above the plot. Each panel retains the same structure. Empty plots show “等待 USB 采样” and the enumeration → selection → connection path. Flags use the framework warning role.

### Linked inspection

All four panels share the time range and cursor fraction. Pointer-anchored wheel zoom and drag pan freeze the snapshot while acquisition continues. The inspection row exposes pause/resume and return-to-live. Keyboard +/− zoom, left/right pan and Escape return-to-live remain available. Chart areas are focusable and describe their channel, units and interaction.

### Statistics strip

Keep bucket bounds, sample count, maximum, minimum and median in the reserved two-line strip. Show an explicit no-data state for empty buckets. Statistics use raw samples; drawing reduction does not redefine the readings.

### Persistent metadata

The footer retains transport/measurement messages, drop and decode counters, protocol errors and threshold. Error state uses the framework danger role. Synthetic demonstration data carries a persistent explicit label identifying it as non-sensor data.

Native components have no shipped HTML/CSS snippets. The sidecar deliberately omits browser component previews and mobile breakpoints rather than fabricating an alternate implementation.

## Do's and Don'ts

### Do:
- **Do** preserve the confirmed Xianii dark palette and native Chinese interface.
- **Do** keep the four channel time axes and inspection cursor linked.
- **Do** retain explicit channel names, units, quality counters and persistent errors.
- **Do** reserve statistics space below each plot and preserve native focus and disabled states.

### Don't:
- **Don't** identify channels by color alone.
- **Don't** cover measurements with statistics tooltips or move plot geometry during inspection.
- **Don't** present synthetic demonstration data as sensor measurements.
- **Don't** introduce a web styling layer or ornamental animation into this native surface.

### Workspace tabs

The two workspace tabs use native `TabsBase` semantics with equal flex widths and a 36 DIP tab height. The strip has a surface background, 1 DIP border, 8 DIP radius and 4 DIP inset/gap. The selected tab has an accent fill and accent-text label; the inactive tab uses muted text and a border-color hover fill. Each tab has a 5 DIP radius. Keep native tab roles, keyboard selection and focus indicators.
