package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"
)

func triggerConfig(low uint32, mask byte, reference uint32, enter, exit uint32, delay uint32) ([]byte, bool) {
	if mask == 0 || mask > 15 || mask&(mask-1) != 0 || exit <= enter || exit > 655340 || delay > 1000 || reference > 655340 || low < lowMinHz || low > lowMaxHz {
		return nil, false
	}
	p := make([]byte, 40)
	binary.LittleEndian.PutUint32(p, low)
	binary.LittleEndian.PutUint32(p[4:], reference)
	binary.LittleEndian.PutUint32(p[16:], enter)
	binary.LittleEndian.PutUint32(p[20:], exit)
	// One valid sample confirms ARM/departure; no additional debounce latency.
	binary.LittleEndian.PutUint32(p[28:], delay)
	binary.LittleEndian.PutUint16(p[32:], 1)
	p[34], p[37], p[38] = 2, 1, mask
	for mask>>p[39] != 1 {
		p[39]++
	}
	return p, true
}
func (s *app) testView(c *ui.Context, status deviceStatus, ready bool) {
	b := s.backend
	active := status.mode != 0 || status.state != 0
	one := status.channels != 0 && status.channels&(status.channels-1) == 0
	ch := byte(0)
	if one {
		for status.channels>>ch != 1 {
			ch++
		}
	}
	if s.referenceChannel != status.channels || s.referenceLow != status.low {
		s.referenceValid = false
	}
	ui.Column(c).Grow(1).Gap(16).Children(func() {
		ui.Text(c, "自动触发测试").FontSize(20).Bold()
		ui.Text(c, "仅支持单通道：对准 ARM 颜色 → Set ARM Color → Test → 等待颜色偏离 → MCU 延时后左键点击；连续测试会重新等待 ARM 颜色").TextColor(s.theme.TextMuted)
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			if ui.Button(c, "Set ARM Color").Tooltip("使用最近 500 ms 的平均值和最大最小值自动标定；保留噪声余量和迟滞间隔，数值可手动微调").Disabled(!ready || !one || active).Clicked() {
				b.mu.Lock()
				s.referenceValid = false
				b.message = "无法保存 ARM 颜色：请等待当前通道的有效新样本（非 CLIP/LATE）"
				b.dirty = true
				if b.history.size > 0 && time.Since(b.lastSample) < 200*time.Millisecond {
					s.live = b.history.copyTo(s.live)
					last := s.live[len(s.live)-1]
					mean, low, high, n := recentStats(s.live, int(ch))
					reference, enter, exit, valid := autoArmThresholds(500000/mean, 500000/high, 500000/low)
					if last.channels == status.channels && last.clip&(1<<ch) == 0 && last.flags == 0 &&
						last.micros-s.live[0].micros >= 500000 && n >= 2 && valid {
						s.reference, s.referenceValid = reference, true
						s.referenceChannel, s.referenceLow = status.channels, status.low
						enterUS := math.Ceil(pulseTolerance(s.reference, enter)*10) / 10
						exitUS := max(enterUS+0.1, math.Ceil(pulseTolerance(s.reference, exit)*10)/10)
						s.enterInput = fmt.Sprintf("%.1f", enterUS)
						s.exitInput = fmt.Sprintf("%.1f", exitUS)
						b.message = "ARM 已按最近 500 ms 自动标定；容差与偏离阈值留有迟滞间隔，可手动微调"
					} else {
						b.message = "无法标定：请保持 ARM 颜色稳定，等待至少 500 ms 有效采样后重试"
					}
				}
				b.mu.Unlock()
			}
			reference := "尚未保存 ARM 颜色"
			if s.referenceValid {
				reference = "ARM 基准 " + pulseText(s.reference)
			}
			ui.Text(c, reference)
		})
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "ARM 容差 / μs")
			ui.TextInput(c, &s.enterInput).Width(80).Label("ARM 容差，单位 μs").Tooltip("换算至固件频率阈值；长脉宽侧边界不对称").Disabled(active)
			ui.Text(c, "偏离阈值 / μs")
			ui.TextInput(c, &s.exitInput).Width(80).Label("偏离阈值，单位 μs").Disabled(active)
			ui.Text(c, "Delay / ms")
			ui.TextInput(c, &s.delayInput).Width(80).Label("延时，0 到 1000 ms").Disabled(active)
		})
		enter, e1 := strconv.ParseFloat(s.enterInput, 64)
		exit, e2 := strconv.ParseFloat(s.exitInput, 64)
		delay, e3 := strconv.ParseUint(s.delayInput, 10, 32)
		enterRaw, exitRaw := pulseThreshold(s.reference, enter), pulseThreshold(s.reference, exit)
		config, valid := triggerConfig(status.low, status.channels, s.reference, enterRaw, exitRaw, uint32(delay))
		valid = valid && enterRaw <= 655340 && exitRaw <= 655340 && enter >= 0 && exit > enter && !math.IsNaN(enter) && !math.IsNaN(exit) && !math.IsInf(enter, 0) && !math.IsInf(exit, 0)
		valid = valid && e1 == nil && e2 == nil && e3 == nil
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			label := "Test"
			if status.mode == 1 {
				label = "取消 Test"
			}
			if ui.PrimaryButton(c, label).Disabled(!ready || active && status.mode != 1 || !active && (!valid || !s.referenceValid)).Clicked() {
				mode := byte(1)
				if active {
					mode = 0
				}
				b.test(config, mode)
			}
			continuous := "连续测试"
			if status.mode == 2 {
				continuous = "停止连续测试"
			}
			if ui.Button(c, continuous).Disabled(!ready || active && status.mode != 2 || !active && (!valid || !s.referenceValid)).Clicked() {
				mode := byte(2)
				if active {
					mode = 0
				}
				b.test(config, mode)
			}
			states := []string{"等待 ARM 颜色", "Armed · 等待颜色偏离", "Delay · MCU 延时中", "发送左键按下", "左键按住", "释放左键"}
			text := "就绪 · 每次 Test 只点击一次"
			if active && int(status.state) < len(states) {
				text = states[status.state]
				if status.mode == 2 {
					text = "连续测试 · " + text
				}
			}
			ui.Text(c, text).TextColor(s.theme.Accent)
		})
		if !one {
			ui.Text(c, "请只开启一个采集通道；允许全部关闭，但无法开始测试").TextColor(s.theme.Warning)
		}
		if !valid && one {
			ui.Text(c, "容差须为有效脉宽且小于基准，偏离阈值须更大；Delay 为 0–1000 ms 整数").TextColor(s.theme.Warning)
		}
		ui.Text(c, "最近 500 ms · 有效样本统计 / μs").Bold()
		for channel, name := range []string{"R", "G", "B", "Clear"} {
			if status.channels&(1<<channel) == 0 {
				continue
			}
			mean, low, high, n := recentStats(s.live, channel)
			text := name + " · 等待有效采样"
			if n > 0 {
				text = fmt.Sprintf("%s    平均 %.1f    最大 %.1f    最小 %.1f    峰峰值 %.1f    n=%d", name, mean, high, low, high-low, n)
			}
			ui.Text(c, text).FontSize(18)
		}
	})
}

// Use the observed envelope plus noise headroom, with a separate hysteresis gap.
func autoArmThresholds(mean, low, high float64) (reference, enter, exit uint32, valid bool) {
	if math.IsNaN(mean) || math.IsInf(mean, 0) || mean < 0 || low < 0 || high < mean || low > mean || high > 655340 {
		return 0, 0, 0, false
	}
	ref := uint32(math.Round(mean))
	minimum, maximum := uint32(math.Round(low)), uint32(math.Round(high))
	spread := max(ref-minimum, maximum-ref)
	margin := max(uint32(20), (spread+3)/4, (ref+199)/200)
	tolerance := spread + margin
	threshold := tolerance + max(spread, 2*margin)
	if ref > 655340 || threshold > max(ref, 655340-ref) {
		return 0, 0, 0, false
	}
	return ref, tolerance, threshold, true
}

// shortcut: firmware compares symmetric frequency distances; pulse tolerances
// map to the shorter-pulse boundary until firmware supports pulse comparisons.
func pulseTolerance(reference, distance uint32) float64 {
	return pulseUS(reference) - 500000/(float64(reference)+float64(distance))
}
func pulseThreshold(reference uint32, tolerance float64) uint32 {
	width := pulseUS(reference)
	if tolerance < 0 || tolerance >= width || math.IsNaN(width) || math.IsNaN(tolerance) || math.IsInf(tolerance, 0) {
		return ^uint32(0)
	}
	value := math.Round(500000/(width-tolerance) - float64(reference))
	if value > 655340 {
		return ^uint32(0)
	}
	return uint32(value)
}
