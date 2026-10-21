package main

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

func (s *app) syncPreferences(status deviceStatus, state string, busy bool, conn *connection) {
	if s.prefs.Version == 0 {
		s.prefs = defaultSettings()
	}
	if s.settingsLoaded && conn != nil && state == "已连接" && s.restoredConnection != conn {
		s.restoredConnection = conn
		s.restoring = true
		s.restoreDeadline = time.Now().Add(5 * time.Second)
		s.backend.applyAcquisition(s.prefs.Low, s.prefs.Channels)
		return
	}
	if s.restoring {
		if !busy && status.low == s.prefs.Low && status.channels == s.prefs.Channels {
			s.restoring = false
			s.inputInitialized = false
		}
		if state == "异常" || state == "未连接" || time.Now().After(s.restoreDeadline) {
			s.restoring = false
			s.settingsError = "恢复采集设置失败，请手动应用确认"
		}
		return
	}
	if s.calibrationRun == nil && !busy && state == "已连接" && status.low >= lowMinHz {
		s.prefs.Low = status.low
		s.prefs.Channels = status.channels
	}
	_ = s.savePreferences()
}
func (s *app) finishCalibration(message string) {
	r := s.calibrationRun
	if r == nil {
		return
	}
	r.Stage = "恢复设置"
	r.Painted = time.Now()
	s.backend.mu.Lock()
	busy := s.backend.adjusting
	s.backend.mu.Unlock()
	r.RestoreRequested = !busy
	if !busy {
		s.backend.applyAcquisition(r.OldLow, r.OldChannels)
	}
	s.backend.mu.Lock()
	s.backend.message = message
	s.backend.dirty = true
	s.backend.mu.Unlock()
}
func (s *app) advanceCalibration(status deviceStatus, state string, busy bool, serial string) {
	r := s.calibrationRun
	if r == nil {
		return
	}
	if state != "已连接" {
		s.prefs.Low = r.OldLow
		s.prefs.Channels = r.OldChannels
		s.calibrationRun = nil
		s.backend.mu.Lock()
		s.backend.message = "校准中断：设备已断开，原映射已保留"
		s.backend.dirty = true
		s.backend.mu.Unlock()
		return
	}
	switch r.Stage {
	case "准备采集":
		if !busy && status.low == 100 && status.channels == 15 && status.mode == 0 && status.state == 0 {
			r.Stage = "扫描"
			r.ColorStart = time.Now()
			r.Painted = time.Time{}
			s.resume()
		}
		if !r.Painted.IsZero() && time.Since(r.Painted) > 5*time.Second {
			s.finishCalibration("校准准备失败，请检查设备状态")
		}
	case "恢复设置":
		if !busy && !r.RestoreRequested && (status.low != r.OldLow || status.channels != r.OldChannels) {
			r.RestoreRequested = true
			s.backend.applyAcquisition(r.OldLow, r.OldChannels)
			return
		}
		if !busy && status.low == r.OldLow && status.channels == r.OldChannels {
			s.calibrationRun = nil
			s.prefs.Low = r.OldLow
			s.prefs.Channels = r.OldChannels
			s.inputInitialized = false
			_ = s.savePreferences()
		}
		if time.Since(r.Painted) > 5*time.Second {
			s.settingsError = "采集设置恢复失败，请手动应用；校准映射已保留"
			s.prefs.Low = r.OldLow
			s.prefs.Channels = r.OldChannels
			s.calibrationRun = nil
		}
	case "扫描":
		if status.low != 100 || status.channels != 15 || status.mode != 0 || status.state != 0 {
			s.finishCalibration("校准中断：采集条件已改变")
			return
		}
		s.backend.mu.Lock()
		data := s.backend.history.copyTo(nil)
		s.backend.mu.Unlock()
		changed, err := r.collect(data, time.Now())
		if err != nil {
			s.finishCalibration(err.Error())
			return
		}
		if changed && len(r.Map.Nodes) == 125 {
			r.Map.Created = time.Now()
			r.Map.Serial = serial
			if err = r.Map.validate(); err != nil {
				s.finishCalibration("校准失败：" + err.Error())
				return
			}
			old := s.calibration
			s.calibration = &r.Map
			if err = s.savePreferences(); err != nil {
				s.calibration = old
				s.finishCalibration("校准保存失败，原映射已保留")
				return
			}
			s.finishCalibration("校准完成，映射已保存")
		}
	}
}
func (s *app) calibrationView(c *ui.Context, status deviceStatus, ready bool, serial string) {
	r := s.calibrationRun
	rgb := [3]int{255, 255, 255}
	if r != nil && len(r.Map.Nodes) < 125 {
		rgb = targetRGB(len(r.Map.Nodes))
	}
	ui.Row(c).Grow(1).Gap(24).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Width(360).Height(360).Background(rgbColor(rgb)).Label("校准纯色区域").Draw(func(_ *ui.Painter, _ ui.Rect) {
			if r != nil && r.Stage == "扫描" && r.Painted.IsZero() {
				s.backend.mu.Lock()
				last := s.backend.history.elapsed
				started := s.backend.history.started
				age := time.Since(s.backend.lastSample)
				s.backend.mu.Unlock()
				if !started {
					return
				}
				r.Painted = time.Now()
				r.After = last + uint64(max(0, age.Microseconds())) + uint64(calibrationSettle.Microseconds()) + 2*uint64(4*channelSlotUS(100))
			}
		})
		ui.Column(c).Grow(1).Gap(8).Children(func() {
			ui.Text(c, "屏幕颜色校准").FontSize(20).Bold()
			ui.Text(c, "将传感器固定在左侧区域，关闭补光灯。\n保持屏幕亮度、显示设置与环境光不变。").TextColor(s.theme.TextMuted)
			ui.Text(c, colorText(rgb, 0)).Font("monospace").Width(240).Shrink(0).FontSize(20)
			if len(s.live) > 0 {
				last := s.live[len(s.live)-1]
				for ch, name := range []string{"R", "G", "B", "Clear"} {
					text := pulseText(last.values[ch])
					if last.low&(1<<ch) != 0 {
						text = "低于量程"
					}
					ui.Textf(c, "%s  %s", name, text)
				}
			}
			if r == nil {
				ui.Text(c, "125 色混色网格 · 稳定后自动切色，约 3 分钟").TextColor(s.theme.TextMuted)
				if ui.PrimaryButton(c, "开始校准").Disabled(!ready || status.mode != 0 || status.state != 0 || s.demo || s.settingsBlocked).Clicked() {
					s.resume()
					s.referenceValid = false
					s.calibrationRun = &calibrationRun{Map: calibrationMap{LowHz: 100}, OldLow: status.low, OldChannels: status.channels, Stage: "准备采集", Painted: time.Now()}
					s.backend.applyAcquisition(100, 15)
				}
			} else {
				ui.Textf(c, "%s · %d / 125 · 连续六组稳定后切色", r.Stage, len(r.Map.Nodes))
				if ui.Button(c, "取消校准").Disabled(r.Stage == "恢复设置").Clicked() {
					s.finishCalibration("校准已取消，原映射已保留")
				}
			}
			if s.calibration == nil {
				ui.Text(c, "尚无校准映射").TextColor(s.theme.TextMuted)
			} else {
				ui.Textf(c, "映射：%s · %s", s.calibration.Serial, s.calibration.Created.Format("2006-01-02 15:04"))
				if s.calibration.Serial != serial {
					ui.Text(c, "设备不匹配：映射已保留，当前停用").TextColor(s.theme.Warning)
				}
				if s.calibration.Nodes[0].Low != 0 {
					ui.Text(c, "黑场低于 100 Hz，以可测下边界归一化").FontSize(12).TextColor(s.theme.Warning)
				}
			}
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, "重置校准").Disabled(r != nil || s.calibration == nil).Clicked() {
					s.resetConfirm = "校准"
				}
				if ui.Button(c, "重置全部设置").Disabled(r != nil).Clicked() {
					s.resetConfirm = "全部设置"
				}
			})
			if s.resetConfirm != "" {
				ui.Text(c, "确认删除"+s.resetConfirm+"？此操作不可撤销。").TextColor(s.theme.Warning)
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c, "确认重置").Clicked() {
						s.resetPreferences(s.resetConfirm == "全部设置")
					}
					if ui.Button(c, "保留").Clicked() {
						s.resetConfirm = ""
					}
				})
			}
			ui.Text(c, "映射仅适用于校准时的屏幕、位置和亮度；\n低于量程或映射范围外不还原颜色。").FontSize(12).TextColor(s.theme.TextMuted)
		})
	})
}
func rgbColor(rgb [3]int) ui.Color {
	return ui.Hex(fmt.Sprintf("#%02X%02X%02X", rgb[0], rgb[1], rgb[2]))
}
func (s *app) calibrationUsable() bool {
	s.backend.mu.Lock()
	serial := s.backend.serial
	s.backend.mu.Unlock()
	return s.calibration != nil && len(s.calibration.Nodes) == 125 && s.calibration.Serial == serial
}
func (s *app) readingIssue(v sample, mask byte) string {
	s.backend.mu.Lock()
	connected := s.backend.state == "已连接" || s.demo
	fresh := time.Since(s.backend.lastSample) < 2*time.Second
	s.backend.mu.Unlock()
	if !connected {
		return "已断开"
	}
	if !s.paused && !s.demo && !fresh {
		return "等待新采样"
	}
	if v.channels&mask != mask {
		return "通道已关闭"
	}
	if v.low&mask != 0 {
		return "低于量程"
	}
	if v.clip&mask != 0 {
		return "CLIP"
	}
	if v.flags != 0 {
		return "LATE"
	}
	for c := range 4 {
		if mask&(1<<c) != 0 && math.IsNaN(pulseUS(v.values[c])) {
			return "等待有效采样"
		}
	}
	return ""
}
func (s *app) channelResponse(c *ui.Context, v sample, ch int, color ui.Color) {
	if !s.calibrationUsable() {
		return
	}
	label := "相对响应 · " + s.readingIssue(v, 1<<ch)
	fraction := 0.0
	if s.readingIssue(v, 1<<ch) == "" {
		fraction = s.calibration.response(v.values[ch], ch)
		label = fmt.Sprintf("相对响应 %5.1f%% · %s%3d", fraction*100, [4]string{"R", "G", "B", "C"}[ch], int(math.Round(fraction*255)))
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Width(100).Height(6).Background(s.theme.Border).Children(func() { ui.Box(c).Width(float32(100 * fraction)).Height(6).Background(color) })
		ui.Text(c, label).Font("monospace").Width(230).Shrink(0).FontSize(12).TextColor(s.theme.TextMuted)
	})
}
func (s *app) measuredColor(c *ui.Context, serial string) {
	if !s.calibrationUsable() {
		return
	}
	data := s.data()
	if len(data) == 0 {
		return
	}
	v := data[len(data)-1]
	if v.channels&7 != 7 {
		return
	}
	if issue := s.readingIssue(v, 7); issue != "" {
		ui.Text(c, issue).FontSize(12)
		return
	}
	rgb, ok := s.calibration.inverse([3]float64{float64(v.values[0]), float64(v.values[1]), float64(v.values[2])})
	if !ok {
		ui.Text(c, "超出校准范围").FontSize(12)
		return
	}
	ui.Box(c).Width(24).Height(24).Background(rgbColor(rgb)).Label("采集颜色预览")
	if ui.Button(c, colorText(rgb, s.colorFormat)).Font("monospace").Width(200).Shrink(0).Tooltip("点击切换 RGB / HEX / HSL 格式").Clicked() {
		s.colorFormat = (s.colorFormat + 1) % 3
		_ = s.savePreferences()
	}
}
