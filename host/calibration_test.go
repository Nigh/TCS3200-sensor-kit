package main

import (
	"github.com/egoist/mygo/ui"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func syntheticMap() *calibrationMap {
	m := &calibrationMap{Serial: "test", Created: time.Now(), LowHz: 100}
	for i := range 125 {
		rgb := targetRGB(i)
		r, g, b := float64(rgb[0]), float64(rgb[1]), float64(rgb[2])
		n := calibrationNode{RGB: rgb, Hz: [4]float64{1000 + 100*r + 20*g + 10*b, 1200 + 10*r + 100*g + 20*b, 1400 + 20*r + 10*g + 100*b, 1500 + 80*(r+g+b)}, Count: [4]int{6, 6, 6, 6}}
		for c, hz := range n.Hz {
			p := 500000 / hz
			n.Pulse[c] = &p
		}
		m.Nodes = append(m.Nodes, n)
	}
	return m
}
func TestCalibrationInterpolation(t *testing.T) {
	m := syntheticMap()
	if err := m.validate(); err != nil {
		t.Fatal(err)
	}
	for _, rgb := range [][3]int{{0, 0, 0}, {255, 255, 255}, {155, 27, 36}, {64, 192, 128}, {127, 129, 25}} {
		hz, ok := m.forward(rgb)
		if !ok {
			t.Fatal("forward", rgb)
		}
		got, ok := m.inverse([3]float64{hz[0], hz[1], hz[2]})
		if !ok || got != rgb {
			t.Fatal("inverse", rgb, got, ok)
		}
	}
	if _, ok := m.inverse([3]float64{-1, -1, -1}); ok {
		t.Fatal("extrapolated")
	}
	m.Nodes[0].Low = 15
	m.Nodes[0].Pulse = [4]*float64{}
	if _, ok := m.inverse([3]float64{1000, 1200, 1400}); ok {
		t.Fatal("censored node used")
	}
	for i := range m.Nodes {
		m.Nodes[i].Hz[1] = m.Nodes[i].Hz[0]
		p := 500000 / m.Nodes[i].Hz[1]
		m.Nodes[i].Pulse[1] = &p
	}
	if err := m.validate(); err == nil {
		t.Fatal("degenerate map accepted")
	}
}
func TestCalibrationSampleIsolation(t *testing.T) {
	now := time.Now()
	r := calibrationRun{Map: calibrationMap{LowHz: 100}, Painted: now.Add(-2 * time.Second), After: 2_000_000}
	data := []sample{{sequence: 1, micros: 1_999_999, channels: 15, values: [4]uint32{9999, 9999, 9999, 9999}}}
	for i := range 8 {
		v := sample{sequence: uint32(i + 2), micros: 2_000_000 + uint64(i)*160160, channels: 15, values: [4]uint32{1000, 2000, 3000, 4000}}
		if i >= 2 {
			v.low = 1
			v.values[0] = 0
		}
		data = append(data, v)
	}
	data = append(data, sample{sequence: 10, micros: 4_000_000, channels: 15, flags: 1})
	changed, err := r.collect(data, now)
	if err != nil || !changed || len(r.Map.Nodes) != 1 {
		t.Fatal(changed, err, r.Map.Nodes)
	}
	n := r.Map.Nodes[0]
	if n.Hz[1] != 2000 || n.Count[0] != 6 || n.Low != 1 || n.Pulse[0] != nil {
		t.Fatal(n)
	}
	r.Painted = now
	r.After = 5_000_000
	if changed, _ := r.collect(data, now); changed {
		t.Fatal("stale data collected")
	}
	if _, err := r.collect(nil, now.Add(6*time.Second)); err == nil {
		t.Fatal("missing timeout")
	}
}
func TestSettingsRoundtripAndProtection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "settings.json")
	p := defaultSettings()
	p.Low = 100
	p.Channels = 7
	p.Format = 2
	p.Calibration = syntheticMap()
	if err := writeSettings(path, p); err != nil {
		t.Fatal(err)
	}
	got, err := readSettings(path)
	if err != nil || got.Low != 100 || got.Calibration.Nodes[31].RGB != targetRGB(31) {
		t.Fatal(got, err)
	}
	s := newApp(newBackend())
	s.settingsPath = path
	s.prefs = got
	s.calibration = got.Calibration
	s.resetPreferences(false)
	got, err = readSettings(path)
	if err != nil || got.Calibration != nil || got.Low != 100 {
		t.Fatal("reset calibration", err, got.Low)
	}
	s.resetPreferences(true)
	got, err = readSettings(path)
	if err != nil || got.Low != 15000 || got.Channels != 15 {
		t.Fatal("reset all", got, err)
	}
	broken := []byte(`{"Version":999}`)
	if err = os.WriteFile(path, broken, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = readSettings(path); err == nil {
		t.Fatal("unknown version accepted")
	}
	s.settingsBlocked = true
	s.calibration = syntheticMap()
	if err = s.savePreferences(); err == nil {
		t.Fatal("overwrote unsupported file")
	}
	bytes, _ := os.ReadFile(path)
	if string(bytes) != string(broken) {
		t.Fatal("file damaged")
	}
	s.settingsBlocked = false
	s.settingsPath = filepath.Join(path, "unwritable")
	old := s.calibration
	s.resetPreferences(false)
	if s.calibration != old {
		t.Fatal("failed save removed map")
	}
}
func TestCalibrationUIAndColorFormats(t *testing.T) {
	s := testApp(t, true)
	s.backend.serial = "test"
	s.calibration = syntheticMap()
	s.tab = 2
	u := ui.NewTester(s.view, 1080, 760)
	square, ok := u.Find("校准纯色区域")
	if !ok || square.W != 360 || square.H != 360 || square.X+square.W > 1080 {
		t.Fatal(square)
	}
	for _, text := range []string{"开始校准", "重置校准", "重置全部设置", "125 色"} {
		if !u.HasText(text) {
			t.Fatal(text)
		}
	}
	saveScreen(t, "calibration-minimum", u)
	u.SetSize(1280, 900)
	saveScreen(t, "calibration-desktop", u)
	rgb := [3]int{155, 27, 36}
	hz, _ := s.calibration.forward(rgb)
	v := sample{sequence: 1, micros: 1000, channels: 15}
	for ch := range 4 {
		v.values[ch] = uint32(math.Round(hz[ch]))
	}
	s.backend.history = history{}
	s.backend.history.add(v)
	s.tab = 0
	u.Frame()
	if !u.HasText("相对响应") || !u.HasText("RGB(155, 27, 36)") {
		t.Fatal(u.Texts())
	}
	if err := u.Click("RGB(155, 27, 36)"); err != nil {
		t.Fatal(err)
	}
	if !u.HasText("#9B1B24") {
		t.Fatal(u.Texts())
	}
	if err := u.Click("#9B1B24"); err != nil {
		t.Fatal(err)
	}
	if !u.HasText("HSL(") {
		t.Fatal(u.Texts())
	}
	u.SetSize(1080, 760)
	saveScreen(t, "calibrated-waveform", u)
	s.backend.serial = "other"
	u.Frame()
	if u.HasText("相对响应") {
		t.Fatal("used another device's map")
	}
	if colorText([3]int{0, 0, 0}, 2) != "HSL(  0,  0%,  0%)" || colorText([3]int{255, 255, 255}, 2) != "HSL(  0,  0%,100%)" {
		t.Fatal("HSL endpoints")
	}
}
func TestCalibrationCancellationRestoresAcquisition(t *testing.T) {
	b := newBackend()
	p := newFake()
	b.open = func(string) (port, error) { return p, nil }
	b.connect("fake")
	waitFor(t, func() bool { return stateIs(b, "已连接") })
	defer b.disconnect()
	s := newApp(b)
	s.calibration = syntheticMap()
	old := s.calibration
	s.calibrationRun = &calibrationRun{Map: calibrationMap{LowHz: 100}, OldLow: 15000, OldChannels: 15, Stage: "准备采集", Painted: time.Now()}
	b.applyAcquisition(100, 15)
	waitFor(t, func() bool { b.mu.Lock(); defer b.mu.Unlock(); return !b.adjusting && b.status.low == 100 })
	s.finishCalibration("取消")
	waitFor(t, func() bool { b.mu.Lock(); defer b.mu.Unlock(); return !b.adjusting && b.status.low == 15000 })
	b.mu.Lock()
	status := b.status
	b.mu.Unlock()
	s.advanceCalibration(status, "已连接", false, "test")
	if s.calibrationRun != nil || s.calibration != old {
		t.Fatal("cancel replaced map or failed restoration")
	}
}

func TestFullCalibrationAndDisconnect(t *testing.T) {
	now := time.Now()
	r := calibrationRun{Map: calibrationMap{LowHz: 100}}
	source := syntheticMap()
	for i, n := range source.Nodes {
		r.Painted = now.Add(-2 * time.Second)
		r.ColorStart = r.Painted
		r.After = uint64(i) * 10_000_000
		var data []sample
		for k := range 8 {
			v := sample{sequence: uint32(i*8 + k), micros: r.After + uint64(k)*160160, channels: 15}
			for c := range 4 {
				v.values[c] = uint32(n.Hz[c])
			}
			data = append(data, v)
		}
		changed, err := r.collect(data, now)
		if err != nil || !changed {
			t.Fatal(i, changed, err)
		}
	}
	if err := r.Map.validate(); err != nil {
		t.Fatal(err)
	}
	zero := [4]calibrationNode{}
	if _, ok := barycentric(zero, [3]float64{}); ok {
		t.Fatal("zero volume tetrahedron accepted")
	}
	s := newApp(newBackend())
	s.calibration = source
	s.calibrationRun = &r
	r.OldLow = 12345
	r.OldChannels = 3
	s.advanceCalibration(deviceStatus{}, "异常", false, "test")
	if s.calibration != source || s.calibrationRun != nil || s.prefs.Low != 12345 {
		t.Fatal("disconnect lost map or settings")
	}
}

func TestSavedAcquisitionRestoresWithoutStartingTest(t *testing.T) {
	b := newBackend()
	p := newFake()
	b.open = func(string) (port, error) { return p, nil }
	b.connect("fake")
	waitFor(t, func() bool { return stateIs(b, "已连接") })
	defer b.disconnect()
	s := newApp(b)
	s.settingsLoaded = true
	s.prefs = defaultSettings()
	s.prefs.Low = 100
	s.prefs.Channels = 7
	s.reference = 12345
	b.mu.Lock()
	status, conn := b.status, b.conn
	b.mu.Unlock()
	s.syncPreferences(status, "已连接", false, conn)
	waitFor(t, func() bool {
		b.mu.Lock()
		defer b.mu.Unlock()
		return !b.adjusting && b.status.low == 100 && b.status.channels == 7
	})
	b.mu.Lock()
	status = b.status
	b.mu.Unlock()
	s.syncPreferences(status, "已连接", false, conn)
	if s.restoring || s.referenceValid || status.mode != 0 {
		t.Fatal("restore did not finish safely")
	}
}

func TestCancelDuringPreparationStillRestores(t *testing.T) {
	b := newBackend()
	p := newFake()
	b.open = func(string) (port, error) { return p, nil }
	b.connect("fake")
	waitFor(t, func() bool { return stateIs(b, "已连接") })
	defer b.disconnect()
	s := newApp(b)
	s.calibrationRun = &calibrationRun{OldLow: 15000, OldChannels: 15, Stage: "准备采集"}
	b.mu.Lock()
	b.adjusting = true
	b.mu.Unlock()
	s.finishCalibration("取消")
	if s.calibrationRun.RestoreRequested {
		t.Fatal("restoration attempted during pending command")
	}
	b.mu.Lock()
	b.adjusting = false
	b.status.low = 100
	p.low = 100
	status := b.status
	b.mu.Unlock()
	s.advanceCalibration(status, "已连接", false, "test")
	waitFor(t, func() bool { b.mu.Lock(); defer b.mu.Unlock(); return !b.adjusting && b.status.low == 15000 })
}

func TestCalibrationWaitsForStableWindow(t *testing.T) {
	now := time.Now()
	r := calibrationRun{Painted: now.Add(-time.Second)}
	for i := range 12 {
		hz := uint32(1000)
		if i < 6 && i%2 == 0 {
			hz = 1200
		}
		v := sample{sequence: uint32(i), micros: uint64(i) * 160160, channels: 15, values: [4]uint32{hz, 2000, 3000, 4000}}
		done, err := r.collect([]sample{v}, now)
		if err != nil {
			t.Fatal(err)
		}
		if done {
			if i < 10 || r.Map.Nodes[0].Hz[0] != 1000 {
				t.Fatal("unstable window accepted", i)
			}
			return
		}
	}
	t.Fatal("stable window did not complete")
}

func TestCalibrationStableFastPath(t *testing.T) {
	now := time.Now()
	r := calibrationRun{Painted: now.Add(-calibrationSettle)}
	for i := range 6 {
		done, err := r.collect([]sample{{sequence: uint32(i), micros: uint64(i) * 160160, channels: 15, values: [4]uint32{1000, 2000, 3000, 4000}}}, now)
		if err != nil || done != (i == 5) {
			t.Fatal(i, done, err)
		}
	}
}
