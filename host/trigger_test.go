package main

import (
	"encoding/binary"
	"github.com/egoist/mygo/ui"
	"math"
	"testing"
	"time"
)

func TestSingleChannelTriggerConfigAndStats(t *testing.T) {
	for ch := range 4 {
		mask := byte(1 << ch)
		p, ok := triggerConfig(4000, mask, 1234, 10, 30, 1000)
		if !ok || len(p) != 30 || p[28] != mask || p[29] != byte(ch) || p[24] != 2 || p[27] != 1 || binary.LittleEndian.Uint32(p[18:]) != 1000 || binary.LittleEndian.Uint16(p[4:]) != 1234 {
			t.Fatal(p, ok)
		}
		if maskedRateHz(20000, mask) != 4000 {
			t.Fatal("single channel cadence")
		}
	}
	for _, mask := range []byte{0, 3, 15, 16} {
		if _, ok := triggerConfig(4000, mask, 0, 10, 30, 0); ok {
			t.Fatal("invalid selection", mask)
		}
	}
	if _, ok := triggerConfig(4000, 1, 0, 30, 30, 0); ok {
		t.Fatal("hysteresis")
	}
	if _, ok := triggerConfig(4000, 1, 0, 10, 30, 1001); ok {
		t.Fatal("delay")
	}
	if maskedRateHz(4000, 0) != 0 || maskedRateHz(20000, 3) != 2000 {
		t.Fatal("masked rate")
	}
	data := []sample{
		{micros: 0, channels: 1, values: [4]uint16{60000}},
		{micros: 500000, channels: 1, values: [4]uint16{100}},
		{micros: 600000, channels: 1, values: [4]uint16{200}},
		{micros: 700000, channels: 0, values: [4]uint16{9000}},
		{micros: 800000, channels: 1, clip: 1, values: [4]uint16{65535}},
		{micros: 900000, channels: 1, flags: 1, values: [4]uint16{9000}},
		{micros: 1000000, channels: 1, values: [4]uint16{300}},
	}
	mean, low, high, n := recentStats(data, 0)
	if n != 3 || mean != 2 || low != 1 || high != 3 {
		t.Fatal(mean, low, high, n)
	}
}
func TestTriggerTab(t *testing.T) {
	s := testApp(t, true)
	s.backend.status.channels = 1
	s.backend.status.rate = 4000
	for i := range s.backend.history.data {
		s.backend.history.data[i].channels = 1
	}
	u := ui.NewTester(s.view, 1080, 760)
	left, okLeft := u.Find("波形测量")
	right, okRight := u.Find("自动触发测试")
	if !okLeft || !okRight || math.Abs(float64(left.W-right.W)) > 1 || left.W < 400 {
		t.Fatal("tab widths", left, right)
	}
	if err := u.Click("自动触发测试"); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"Set ARM Color", "Test", "连续测试", "最近 500 ms", "峰峰值"} {
		if !u.HasText(text) {
			t.Fatal(text, u.Texts())
		}
	}
	if _, ok := u.Find("R · 红通道，单位 kHz"); ok {
		t.Fatal("waveform in test tab")
	}
	saveScreen(t, "trigger-minimum", u)
	u.SetSize(1280, 900)
	saveScreen(t, "trigger-desktop", u)
}

func TestTriggerUICommands(t *testing.T) {
	b := newBackend()
	p := newFake()
	b.open = func(string) (port, error) { return p, nil }
	b.connect("fake")
	waitFor(t, func() bool { return stateIs(b, "已连接") })
	defer b.disconnect()
	b.setChannels(0)
	waitFor(t, func() bool {
		b.mu.Lock()
		defer b.mu.Unlock()
		return !b.adjusting && b.status.channels == 0 && b.status.rate == 0
	})
	b.setChannels(8)
	waitFor(t, func() bool { b.mu.Lock(); defer b.mu.Unlock(); return !b.adjusting && b.status.channels == 8 })
	b.mu.Lock()
	b.history.add(sample{sequence: 1, micros: 1000, channels: 8, values: [4]uint16{0, 0, 0, 1200}})
	b.history.add(sample{sequence: 2, micros: 501000, channels: 8, values: [4]uint16{0, 0, 0, 1268}})
	b.lastSample = time.Now()
	b.mu.Unlock()
	s := newApp(b)
	s.invalidateStarted = true
	s.tab = 1
	u := ui.NewTester(s.view, 1080, 760)
	if err := u.Click("Set ARM Color"); err != nil {
		t.Fatal(err)
	}
	if !s.referenceValid || s.reference != 1234 {
		t.Fatal("baseline", s.reference)
	}
	if s.enterInput != "43" || s.exitInput != "77" {
		t.Fatal("automatic thresholds", s.enterInput, s.exitInput)
	}
	if err := u.Click("Test"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { b.mu.Lock(); defer b.mu.Unlock(); return !b.adjusting && b.status.mode == 1 })
	p.mu.Lock()
	if len(p.config) != 30 || p.config[29] != 3 || binary.LittleEndian.Uint16(p.config[4:]) != 1234 {
		t.Error("configuration", p.config)
	}
	p.mu.Unlock()
	u.Frame()
	if err := u.Click("取消 Test"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { b.mu.Lock(); defer b.mu.Unlock(); return !b.adjusting && b.status.mode == 0 })
	u.Frame()
	if err := u.Click("连续测试"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { b.mu.Lock(); defer b.mu.Unlock(); return !b.adjusting && b.status.mode == 2 })
	u.Frame()
	if err := u.Click("停止连续测试"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { b.mu.Lock(); defer b.mu.Unlock(); return !b.adjusting && b.status.mode == 0 })
}

func TestAutomaticArmEnvelope(t *testing.T) {
	for _, v := range []struct{ mean, low, high float64 }{
		{0, 0, 0}, {100, 99.8, 100.3}, {327.67, 327.67, 327.67}, {655.34, 655.34, 655.34},
	} {
		ref, enter, exit, ok := autoArmThresholds(v.mean, v.low, v.high)
		spread := max(math.Abs(float64(ref)-v.low*100), math.Abs(v.high*100-float64(ref)))
		if !ok || float64(enter) <= spread || exit <= enter || exit-enter < 4 {
			t.Fatal(v, ref, enter, exit, ok)
		}
	}
	if _, _, _, ok := autoArmThresholds(327.67, 0, 655.34); ok {
		t.Fatal("unusable full-scale noise accepted")
	}
}
