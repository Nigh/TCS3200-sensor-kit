package main

import (
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func testApp(t *testing.T, populated bool) *app {
	t.Helper()
	b := newBackend()
	s := newApp(b)
	s.invalidateStarted = true
	if populated {
		b.state = "合成演示"
		s.demo = true
		b.message = "合成测试数据 · 非真实传感器读数"
		b.status = deviceStatus{rate: 1000, slot: 250, low: 20000, channels: 15}
		b.usb = usbStatus{1000, 1000}
		for i := range 12000 {
			v := sample{channels: 15, sequence: uint32(i), micros: uint64(i * 1000)}
			for ch := range 4 {
				v.values[ch] = uint16(20000 + ch*5000 + int(4000*math.Sin(float64(i)*.007*float64(ch+1))))
			}
			if i%997 == 0 {
				v.values[0] = 40000
			}
			b.history.add(v)
		}
	}
	return s
}
func saveScreen(t *testing.T, name string, tester *ui.Tester) {
	t.Helper()
	dir := os.Getenv("TCS3200_SCREENSHOTS")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err = png.Encode(f, tester.Image()); err != nil {
		t.Fatal(err)
	}
}
func TestMeasurementUI(t *testing.T) {
	s := testApp(t, true)
	u := ui.NewTester(s.view, 1280, 900)
	for _, name := range []string{"R · 红通道", "G · 绿通道", "B · 蓝通道", "Clear · 透明通道"} {
		if !u.HasText(name) {
			t.Fatal(name)
		}
	}
	if !u.HasText("HID 请求 1000 Hz · 空闲轮询估算 1000 Hz") {
		t.Fatal("HID poll status missing")
	}
	r, ok := u.Find("R · 红通道，单位 kHz")
	if !ok || r.W < 200 || r.H < 100 {
		t.Fatal("plot geometry", r)
	}
	u.Move(r.X+r.W/2, r.Y+r.H/2)
	if !u.HasText("中位数") || !u.HasText("n=") {
		t.Fatalf("hover statistics: r=%+v hover=%v fraction=%v channel=%v texts=%v", r, s.hovering, s.hoverFraction, s.hoverChannel, u.Texts())
	}
	saveScreen(t, "desktop", u)
	if err := u.Click("暂停刷新"); err != nil {
		t.Fatal(err)
	}
	if !s.paused {
		t.Fatal("pause")
	}
	old := s.end
	s.backend.mu.Lock()
	s.backend.history.add(sample{channels: 15, sequence: 12000, micros: 12000000})
	s.backend.mu.Unlock()
	u.Frame()
	if s.end != old {
		t.Fatal("snapshot moved")
	}
	u.Scroll(r.X+r.W/2, r.Y+r.H/2, 0, -100)
	if s.span >= float64(windowUS) {
		t.Fatal("zoom")
	}
	u.Press(r.X+r.W/2, r.Y+r.H/2)
	u.Move(r.X+r.W*.7, r.Y+r.H/2)
	u.Release(r.X+r.W*.7, r.Y+r.H/2)
	beforeKey := s.span
	u.Key(ui.Shift, ui.KeyEqual)
	if s.span >= beforeKey {
		t.Fatal("focused keyboard zoom")
	}
	u.Move(r.X+r.W+40, r.Y+r.H+40)
	u.Frame()
	if s.hovering {
		t.Fatal("hover did not clear")
	}

	if err := u.Click("回到实时"); err != nil {
		t.Fatal(err)
	}
	if s.paused || s.end != 12000000 {
		t.Fatal("resume", s.end)
	}
	u.SetSize(1080, 760)
	saveScreen(t, "minimum", u)
	for _, name := range []string{"R · 红通道，单位 kHz", "G · 绿通道，单位 kHz", "B · 蓝通道，单位 kHz", "Clear · 透明通道，单位 kHz"} {
		r, ok := u.Find(name)
		if !ok || r.X < 0 || r.Y < 0 || r.X+r.W > 1080 || r.Y+r.H > 760 {
			t.Fatal("plot outside window", name, r)
		}
	}
	u.SetScale(2)
	if s.plotPixels[0] < int(r.W) {
		t.Fatal("physical pixels")
	}
}
func TestEmptyUI(t *testing.T) {
	s := testApp(t, false)
	u := ui.NewTester(s.view, 1080, 760)
	if !u.HasText("等待 USB 采样") {
		t.Fatal("empty state")
	}
	saveScreen(t, "empty", u)
}

func TestThresholdClampsAndPreviewsRate(t *testing.T) {
	b := newBackend()
	p := newFake()
	b.open = func(string) (port, error) { return p, nil }
	b.connect("fake")
	waitFor(t, func() bool { return stateIs(b, "已连接") })
	defer b.disconnect()
	s := newApp(b)
	s.invalidateStarted = true
	u := ui.NewTester(s.view, 1080, 760)
	if !u.HasText("当前最大采样率 814 Hz") {
		t.Fatal("device rate missing")
	}
	if err := u.Click("低频阈值，4000 到 600000 Hz"); err != nil {
		t.Fatal(err)
	}
	u.Key(ui.Ctrl, ui.KeyA)
	u.Type("1")
	if !u.HasText("所填最大采样率 240 Hz") {
		t.Fatal("draft rate missing")
	}
	if err := u.Click("TCS3200 测量台"); err != nil {
		t.Fatal(err)
	}
	if s.lowInput != "4000" {
		t.Fatal("blur did not clamp", s.lowInput)
	}
	if err := u.Click("低频阈值，4000 到 600000 Hz"); err != nil {
		t.Fatal(err)
	}
	u.Key(ui.Ctrl, ui.KeyA)
	u.Type("9999999")
	if err := u.Click("应用"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { b.mu.Lock(); defer b.mu.Unlock(); return !b.adjusting && b.status.low == lowMaxHz })
	u.Frame()
	if s.lowInput != "600000" || !u.HasText("当前最大采样率 1000 Hz") {
		t.Fatal("submit did not clamp", s.lowInput)
	}
	saveScreen(t, "threshold", u)
}
func TestVariableCadenceChart(t *testing.T) {
	s := testApp(t, false)
	b := s.backend
	b.state = "合成演示"
	s.demo = true
	b.status = deviceStatus{rate: 240, slot: 1040, low: 4000, channels: 15}
	for i := range 2404 {
		b.history.add(sample{channels: 15, sequence: uint32(i), micros: uint64(i) * 4160, values: [4]uint16{1000, 2000, 3000, 4000}})
	}
	u := ui.NewTester(s.view, 1080, 760)
	for _, pc := range s.plots {
		for _, y := range pc.by {
			if math.IsNaN(float64(y)) {
				t.Fatal("valid low-rate samples broken into gaps")
			}
		}
	}
	if !u.HasText("当前最大采样率 240 Hz") {
		t.Fatal("low rate missing")
	}
	saveScreen(t, "low-rate", u)
}
