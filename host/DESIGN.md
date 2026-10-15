# Measurement workspace

Mode: Operate. Native Linux desktop; no HTML/CSS rendering.

Visual authority: https://github.com/Nigh/xianii-theme (MIT), packages/design-system/tokens.css. Adapt its dark palette, 8 DIP control radius, 14 DIP controls, 12 DIP metadata, 20 DIP headings and spacing in multiples of 4 into ui.Theme. Preserve the theme's neutral background and pink action accent. Channel colors distinguish R/G/B/C but labels always identify channels independently of color.

First viewport: compact connection toolbar above a 2×2 grid of equally sized waveform plots. Each plot shows the channel name, frequency, grid and time ticks; a stable statistics strip avoids tooltip overlap. Footer holds acquisition quality and error recovery information. Minimum window 1080×760; initial window 1280×900.

Interactions: four linked time axes and cursor; wheel zoom with pointer anchor, drag within a frozen snapshot, explicit pause/resume and return-to-live. Paused charts retain their geometry while acquisition continues. Keyboard +/- zoom, left/right pan and Escape return to live. Empty plots state that a USB device must be connected. Errors are persistent text, not transient notifications.

Tokens (sRGB conversion of upstream OKLCH): background #161616, surface #242424, border #404040, text #f2f2f3, muted #b4b4b7, primary #ff9db1, primary text #242424; channel red #ff9db1, green #8bd5a3, blue #8bbdf0, clear #d3b1f4. Main spacing 16, group spacing 8, panel radius 8; plot grid uses a subdued neutral line.

Font: Noto Sans CJK SC for Chinese and UI labels, with the framework's fallback; numerical readings are formatted consistently with explicit units. Native focus rings, disabled states and control labels remain enabled. No ornamental animation or raster assets.
