package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"slices"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

type plotCache struct {
	xs, ys, bx, by []float32
	key            string
}
type app struct {
	backend            *backend
	selected, lowInput string
	inputInitialized   bool
	lowFocused         bool
	epoch              uint64
	live, snapshot     []sample
	paused             bool
	refreshPaused      atomic.Bool
	end, span          float64
	hover              float64
	hoverFraction      float64
	hovering           bool
	hoverChannel       int
	demo               bool
	plotPixels         [4]int
	plotWidths         [4]float32
	drag               bool
	dragX, dragEnd     float64
	plots              [4]plotCache
	scratch            []uint16
	theme              *ui.Theme
	invalidateStarted  bool
	stop               chan struct{}
}

func newApp(b *backend) *app {
	t := ui.DarkTheme()
	t.Background = ui.Hex("#161616")
	t.Surface = ui.Hex("#242424")
	t.Border = ui.Hex("#404040")
	t.Text = ui.Hex("#f2f2f3")
	t.TextMuted = ui.Hex("#b4b4b7")
	t.Accent = ui.Hex("#ff9db1")
	t.AccentText = t.Surface
	t.AccentHover = ui.Hex("#ffb4c2")
	t.Focus = t.Accent
	t.Radius = 8
	t.FontSize = 14
	t.Font = "Noto Sans CJK SC"
	return &app{backend: b, span: float64(windowUS), theme: t, stop: make(chan struct{}), lowInput: "15000"}
}
func (s *app) data() []sample {
	if s.paused {
		return s.snapshot
	}
	return s.live
}
func (s *app) freeze() {
	if !s.paused {
		s.snapshot = append(s.snapshot[:0], s.live...)
		s.paused = true
		s.refreshPaused.Store(true)
	}
}
func (s *app) resume() {
	s.paused = false
	s.refreshPaused.Store(false)
	s.drag = false
	s.hovering = false
	s.span = float64(windowUS)
}
func (s *app) clamp() {
	data := s.data()
	if len(data) == 0 {
		return
	}
	last := float64(data[len(data)-1].micros)
	first := float64(data[0].micros)
	s.end = max(min(first+s.span, last), min(s.end, last))
}
func (s *app) zoom(factor, anchor float64) {
	if len(s.data()) == 0 {
		return
	}
	s.freeze()
	old := s.span
	s.span = max(10000, min(float64(windowUS), old*factor))
	s.end += (s.span - old) * (1 - anchor)
	s.clamp()
}
func (s *app) input(ev ui.InputEvent, width float32, ch int) bool {
	if width <= 0 {
		return false
	}
	switch ev.Kind {
	case ui.InputKeyDown:
		if len(s.data()) == 0 {
			return false
		}
		if ev.Key == ui.KeyEqual && (ev.Mods == 0 || ev.Mods == ui.Shift) {
			s.zoom(.8, .5)
			return true
		}
		if ev.Mods != 0 {
			return false
		}
		switch ev.Key {
		case ui.KeyMinus:
			s.zoom(1.25, .5)
			return true
		case ui.KeyLeft:
			s.freeze()
			s.end -= s.span * .1
			s.clamp()
			return true
		case ui.KeyRight:
			s.freeze()
			s.end += s.span * .1
			s.clamp()
			return true
		}

	case ui.InputScroll:
		s.hoverFraction = max(0, min(1, float64(ev.X/width)))
		s.hoverChannel = ch
		s.hovering = true
		s.zoom(math.Exp(float64(ev.DY)*.003), max(0, min(1, float64(ev.X/width))))
		return true
	case ui.InputPointerDown:
		if ev.Button == 0 && len(s.data()) > 0 {
			s.freeze()
			s.drag = true
			s.dragX = float64(ev.X)
			s.dragEnd = s.end
			return true
		}
	case ui.InputPointerMove:
		s.hoverFraction = max(0, min(1, float64(ev.X/width)))
		s.hoverChannel = ch
		s.hovering = true
		if s.drag {
			s.end = s.dragEnd - (float64(ev.X)-s.dragX)/float64(width)*s.span
			s.clamp()
			return true
		}
		return true
	case ui.InputPointerUp:
		if s.drag {
			s.drag = false
			return true
		}
	}
	return false
}
func (s *app) view(c *ui.Context) {
	c.SetTheme(s.theme)
	if !s.invalidateStarted {
		s.invalidateStarted = true
		invalidate := c.Invalidate
		go func() {
			ticker := time.NewTicker(time.Second / 30)
			defer ticker.Stop()
			for {
				select {
				case <-s.stop:
					return
				case <-ticker.C:
					s.backend.mu.Lock()
					dirty := s.backend.dirty || s.backend.sampleDirty && !s.refreshPaused.Load()
					s.backend.dirty = false
					s.backend.sampleDirty = false
					s.backend.mu.Unlock()
					if dirty {
						invalidate()
					}
				}
			}
		}()
	}
	b := s.backend
	b.mu.Lock()
	devices := slices.Clone(b.devices)
	state, message := b.state, b.message
	status := b.status
	busy, enumerating := b.adjusting, b.enumerating
	errors, drops := b.parserErrors, b.history.drops
	if s.epoch != b.resetID {
		s.epoch = b.resetID
		s.paused = false
		s.refreshPaused.Store(false)
		s.snapshot = s.snapshot[:0]
		s.live = s.live[:0]
		s.inputInitialized = false
		s.hovering = false
		s.drag = false
		s.span = float64(windowUS)
	}
	if !s.paused {
		s.live = b.history.copyTo(s.live)
	}
	b.mu.Unlock()
	if !s.inputInitialized && status.low != 0 {
		s.lowInput = strconv.FormatUint(uint64(status.low), 10)
		s.inputInitialized = true
	}
	if !s.paused && len(s.live) > 0 {
		s.end = float64(s.live[len(s.live)-1].micros)
	}
	options := make([]string, len(devices))
	valid := false
	for i, d := range devices {
		options[i] = d.label
		valid = valid || s.selected == d.label
	}
	if !valid {
		s.selected = ""
		if len(options) > 0 {
			s.selected = options[0]
		}
	}
	if c.Shortcut(0, ui.KeyEscape) {
		s.resume()
	}
	data := s.data()
	ui.Column(c).Fill().Padding(20).Gap(12).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(16).Children(func() {
			ui.Text(c, "TCS3200 测量台").FontSize(22).Bold()
			ui.Text(c, state).TextColor(s.theme.Accent)
			ui.Box(c).Grow(1)
			ui.Text(c, func() string {
				if status.rate == 0 {
					return "当前最大采样率 —"
				}
				return fmt.Sprintf("当前最大采样率 %d Hz", status.rate)
			}()).TextColor(s.theme.TextMuted).FontSize(12)
			ui.Textf(c, "所填最大采样率 %d Hz", maximumRateHz(clampedLowInput(s.lowInput, status.low))).FontSize(12).TextColor(s.theme.TextMuted)
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			label := "枚举设备"
			if enumerating {
				label = "枚举中…"
			}
			if ui.Button(c, label).Disabled(enumerating || s.demo).Clicked() {
				b.scan()
			}
			ui.Select(c, &s.selected, options).Width(330).Label("USB CDC 设备").Disabled(state == "连接中" || state == "已连接" || len(options) == 0)
			if state == "已连接" || state == "连接中" {
				if ui.Button(c, "断开").Clicked() {
					b.disconnect()
				}
			} else if ui.PrimaryButton(c, "连接").Disabled(s.selected == "" || s.demo).Clicked() {
				for _, d := range devices {
					if d.label == s.selected {
						b.connect(d.path)
						break
					}
				}
			}
			ui.Box(c).Grow(1)
			ui.Text(c, "低频阈值 / Hz").TextColor(s.theme.TextMuted)
			input := ui.TextInput(c, &s.lowInput).Width(100).Label("低频阈值，4000 到 600000 Hz").Tooltip("离开输入框或应用时自动限制到 4000–600000 Hz").Disabled(state != "已连接" || busy)
			focused := input.Focused()
			if s.lowFocused && !focused {
				s.lowInput = strconv.FormatUint(uint64(clampedLowInput(s.lowInput, status.low)), 10)
			}
			s.lowFocused = focused
			if ui.Button(c, "应用").Disabled(state != "已连接" || busy || status.mode != 0 || status.state != 0).Tooltip("范围 4000–600000 Hz；应用后已有测试配置失效").Clicked() {
				v := clampedLowInput(s.lowInput, status.low)
				s.lowInput = strconv.FormatUint(uint64(v), 10)
				b.setLow(v)
			}
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			label := "暂停刷新"
			if s.paused {
				label = "恢复实时"
			}
			if ui.Button(c, label).Disabled(len(data) == 0).Clicked() {
				if s.paused {
					s.resume()
				} else {
					s.freeze()
				}
			}
			if ui.Button(c, "回到实时").Disabled(!s.paused && s.span == float64(windowUS)).Clicked() {
				s.resume()
			}
			mode := "实时"
			if s.paused {
				mode = "快照"
			}
			ui.Textf(c, "%s · %.3g s", mode, s.span/1e6).TextColor(s.theme.Accent)
			ui.Box(c).Grow(1)
			ui.Text(c, "滚轮缩放 · 拖动平移 · +/- 缩放 · ←/→ 平移 · Esc 实时").FontSize(12).TextColor(s.theme.TextMuted)
		})
		ui.Grid(c).ColumnTracks(ui.Fr(1), ui.Fr(1)).RowTracks(ui.Fr(1), ui.Fr(1)).Grow(1).MinHeight(400).Gap(12).Children(func() {
			for ch := range 4 {
				s.plot(c.Key(ch), ch)
			}
		})
		ui.Text(c, message).TextColor(func() ui.Color {
			if state == "异常" {
				return s.theme.Danger
			}
			return s.theme.TextMuted
		}()).FontSize(13)
		threshold := "—"
		if status.low > 0 {
			threshold = fmt.Sprintf("%.2f kHz", float64(status.low)/1000)
		}
		ui.Textf(c, "采样丢组 %d · 设备采集丢组 %d · 发送丢组 %d · 解码错误 %d · 设备协议错误 %d · 阈值 %s", drops, status.captureDrops, status.streamDrops, errors, status.parserErrors, threshold).FontSize(12).TextColor(s.theme.TextMuted)
	})
}
func (s *app) plot(c *ui.Context, ch int) {
	names := [4]string{"R · 红通道", "G · 绿通道", "B · 蓝通道", "Clear · 透明通道"}
	colors := [4]ui.Color{ui.Hex("#ff9db1"), ui.Hex("#8bd5a3"), ui.Hex("#8bbdf0"), ui.Hex("#d3b1f4")}
	data := s.data()
	if s.hovering {
		s.hover = s.end - s.span + s.hoverFraction*s.span
	}
	lo, hi := s.end-s.span, s.end
	pc := &s.plots[ch]
	var last sample
	if len(data) > 0 {
		last = data[len(data)-1]
	}
	maxY := float64(1)
	for _, v := range data {
		if float64(v.micros) >= lo && float64(v.micros) <= hi {
			maxY = max(maxY, float64(v.values[ch])/100)
		}
	}
	yscale := chart.NewLinear(0, maxY*1.08).Nice(4)
	xticks := chart.NewLinear(-s.span/1e6, 0).Ticks(5)
	ui.Column(c).Padding(14).Gap(8).Background(s.theme.Surface).Radius(8).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, names[ch]).TextColor(colors[ch]).Bold()
			ui.Box(c).Grow(1)
			value := "— kHz"
			if len(data) > 0 {
				value = fmt.Sprintf("%.2f kHz", float64(last.values[ch])/100)
			}
			ui.Text(c, value).FontSize(20)
			flags := ""
			if len(data) > 0 {
				if last.low&(1<<ch) != 0 {
					flags += " LOW"
				}
				if last.clip&(1<<ch) != 0 {
					flags += " CLIP"
				}
				if last.flags&1 != 0 {
					flags += " LATE"
				}
			}
			ui.Text(c, flags).FontSize(12).TextColor(s.theme.Warning)
		})
		var statsText = "悬停查看时间桶与样本数\nMax / Min / 中位数 · kHz"
		chart.Plot(c, chart.Axes{Left: yscale.Ticks(4), Bottom: xticks}, func(area ui.Element) {
			area.Label(names[ch] + "，单位 kHz").Description("最近十秒；滚轮缩放，拖动平移，方向键浏览").Focusable()
			area.HandleInput(func(ev ui.InputEvent) bool { return s.input(ev, s.plotWidths[ch], ch) })
			_, _, over := area.PointerPosition()
			if !over && s.hoverChannel == ch && s.hovering {
				s.hovering = false
				c.Invalidate()
			}

			// The painter scale sets physical pixel bins; layout-time stats use the last drawn width/scale.
			if s.hovering && s.hover >= lo && s.hover <= hi {
				pixelWidth := max(1, s.plotPixels[ch])
				bin := math.Min(float64(pixelWidth-1), math.Floor((s.hover-lo)/s.span*float64(pixelWidth)))
				a := lo + bin*s.span/float64(pixelWidth)
				z := lo + (bin+1)*s.span/float64(pixelWidth)
				if int(bin) == pixelWidth-1 {
					z = math.Nextafter(z, math.Inf(1))
				}
				st, scratch := bucketStats(data, ch, a, z, s.scratch)
				s.scratch = scratch
				if st.count == 0 {
					statsText = "当前时间桶无数据\nMax — · Min — · 中位数 — kHz"
				} else {
					statsText = fmt.Sprintf("%+.4f…%+.4f s · n=%d\nMax %.2f · Min %.2f · 中位数 %.2f kHz", (a-hi)/1e6, (z-hi)/1e6, st.count, st.max, st.min, st.median)
				}
			}
			area.Draw(func(p *ui.Painter, r ui.Rect) {
				s.plotWidths[ch] = r.W
				pixels := max(1, int(math.Ceil(float64(r.W*p.Scale()))))
				s.plotPixels[ch] = pixels
				for _, tick := range yscale.Ticks(4) {
					y := chart.Y(r, tick.At)
					p.Line(r.X, y, r.X+r.W, y, 1, ui.Hex("#343434"))
				}
				for _, tick := range xticks {
					x := chart.X(r, tick.At)
					p.Line(x, r.Y, x, r.Y+r.H, 1, ui.Hex("#303030"))
				}
				key := fmt.Sprintf("%d/%d/%d/%f/%f/%f/%d", s.epoch, len(data), last.sequence, lo, hi, maxY, pixels)
				if pc.key != key {
					pc.key = key
					pc.xs, pc.ys = pc.xs[:0], pc.ys[:0]
					first := true
					var prev sample
					for _, v := range data {
						if float64(v.micros) < lo || float64(v.micros) > hi {
							continue
						}
						x := float32((float64(v.micros) - lo) / s.span)
						if !first && (v.sequence-prev.sequence != 1 || v.micros <= prev.micros) {
							pc.xs = append(pc.xs, x)
							pc.ys = append(pc.ys, float32(math.NaN()))
						}
						pc.xs = append(pc.xs, x)
						pc.ys = append(pc.ys, yscale.At(float64(v.values[ch])/100))
						prev = v
						first = false
					}
					pc.bx, pc.by = minmax(pc.bx, pc.by, pc.xs, pc.ys, pixels)
				}
				p.Clip(r, 0, func() {
					p.StrokePath(chart.Line(r, pc.bx, pc.by, chart.CurveLinear), 1.5, colors[ch])
					if s.hovering && s.hover >= lo && s.hover <= hi {
						x := chart.X(r, float32((s.hover-lo)/s.span))
						p.Line(x, r.Y, x, r.Y+r.H, 1, s.theme.TextMuted)
					}
				})
			})
			if len(data) == 0 {
				ui.Column(c).Fill().Center().Children(func() {
					ui.Text(c, "等待 USB 采样").TextColor(s.theme.TextMuted)
					ui.Text(c, "枚举设备 → 选择 → 连接").FontSize(12).TextColor(s.theme.TextMuted)
				})
			}
		}).Grow(1).MinHeight(120).Gap(6)
		ui.Text(c, statsText).FontSize(12).TextColor(s.theme.TextMuted).MinHeight(36)
	})
}

func main() {
	demo := flag.Bool("demo", false, "使用明确标注的合成 1 kHz 四通道信号，不连接 USB")
	flag.Parse()
	b := newBackend()
	s := newApp(b)
	if *demo {
		s.demo = true
		b.state = "合成演示"
		startDemo(b, s.stop)
	}
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{Title: "TCS3200 测量台", Width: 1280, Height: 900, MinWidth: 1080, MinHeight: 760, Content: ui.View(s.view)})
	})
	mygo.App.OnQuit(func() { close(s.stop); b.disconnect() })
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
func startDemo(b *backend, stop <-chan struct{}) {
	b.message = "合成演示数据 · 非真实传感器读数"
	b.status = deviceStatus{rate: 1000, slot: 250, low: 20000}
	b.dirty = true
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		start := time.Now()
		var seq uint32
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				target := uint32(time.Since(start) / time.Millisecond)
				b.mu.Lock()
				for ; seq < target; seq++ {
					v := sample{sequence: seq, micros: uint64(seq) * 1000}
					for ch := range 4 {
						x := float64(seq) / 1000
						value := 19000 + float64(ch)*7000 + 4000*math.Sin(x*float64(ch+1)*3) + 500*math.Sin(x*157)
						if seq%997 == uint32(ch*31) {
							value += 6000
						}
						v.values[ch] = uint16(value)
					}
					b.history.add(v)
				}
				b.sampleDirty = true
				b.mu.Unlock()
			}
		}
	}()
}
