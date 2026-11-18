package main

import (
	"fmt"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

func spectrumSamples(count int, rate, frequency float64) []sample {
	data := make([]sample, count)
	for i := range data {
		pulse := 100 + 20*math.Sin(2*math.Pi*frequency*float64(i)/rate)
		value := uint32(math.Round(500000 / pulse))
		data[i] = sample{sequence: uint32(i), micros: uint64(math.Round(float64(i) * 1e6 / rate)), channels: 15, values: [4]uint32{value, value, value, value}}
	}
	return data
}

func TestSpectrumFrequencyAndAmplitude(t *testing.T) {
	for _, tc := range []struct {
		rate, frequency float64
		count           int
	}{
		{4000, 125, 40000},
		{1000, 123.4, 1024},
		{6.25, .78125, 62},
	} {
		t.Run(fmt.Sprint(tc.rate), func(t *testing.T) {
			data := spectrumSamples(tc.count, tc.rate, tc.frequency)
			var sp spectrum
			sp.update(data, 0, 0, 1e8, 0)
			if !sp.hasPeak || math.Abs(sp.peakHz-tc.frequency) > sp.rate/float64(sp.n)*.1 || math.Abs(sp.maximum-20) > 3 {
				t.Fatalf("frequency/amplitude: n=%d peak=%v %.4f Hz %.3f μs, message=%s", sp.n, sp.hasPeak, sp.peakHz, sp.maximum, sp.message)
			}
			if sp.n > maxFFT || len(sp.work) > maxFFT || math.Abs(sp.rate-tc.rate) > .01 {
				t.Fatal("unbounded FFT or incorrect timestamp rate", sp.n, len(sp.work), sp.rate)
			}
		})
	}
}

func TestSpectrumUsesRecentViewportSamples(t *testing.T) {
	data := spectrumSamples(40000, 4000, 125)
	old := spectrumSamples(len(data)-maxFFT, 4000, 62.5)
	copy(data, old)
	var sp spectrum
	sp.update(data, 0, 0, 10e6, 0)
	if sp.n != maxFFT || math.Abs(sp.peakHz-125) > .1 {
		t.Fatal("FFT did not use bounded recent tail", sp.n, sp.peakHz)
	}
	sp.update(data, 0, 0, 3e6, 0)
	if math.Abs(sp.peakHz-62.5) > .1 {
		t.Fatal("FFT ignored frozen viewport", sp.peakHz)
	}
	sp.update(data, 0, 1e8, 2e8, 0)
	if sp.n != 0 || sp.hasPeak {
		t.Fatal("empty viewport retained stale spectrum")
	}
}

func TestSpectrumRejectsInvalidRunsAndNoise(t *testing.T) {
	for _, kind := range []string{"LOW", "CLIP", "LATE", "disabled", "gap", "jitter", "DC", "noise"} {
		t.Run(kind, func(t *testing.T) {
			data := spectrumSamples(512, 1000, 125)
			last := &data[len(data)-1]
			switch kind {
			case "LOW":
				last.low = 1
			case "CLIP":
				last.clip = 1
			case "LATE":
				last.flags = 1
			case "disabled":
				last.channels = 14
			case "gap":
				for i := len(data) - 16; i < len(data); i++ {
					data[i].sequence += 10
				}
			case "jitter":
				for i := range data {
					if i%2 != 0 {
						data[i].micros += 400
					}
				}
			case "DC":
				for i := range data {
					data[i].values[0] = 5000
				}
			case "noise":
				state := uint32(1)
				for i := range data {
					state = state*1664525 + 1013904223
					pulse := 100 + 10*(float64(state)/float64(math.MaxUint32)-.5)
					data[i].values[0] = uint32(math.Round(500000 / pulse))
				}
			}
			var sp spectrum
			sp.update(data, 0, 0, 1e6, 0)
			if sp.hasPeak {
				t.Fatal("invalid or unstructured signal produced peak", sp.peakHz)
			}
			if kind != "DC" && kind != "noise" && sp.n != 0 {
				t.Fatal("FFT bridged invalid samples", sp.n)
			}
		})
	}
}

func TestSpectrumUI(t *testing.T) {
	s := testApp(t, false)
	s.demo = true
	s.backend.state = "合成演示"
	s.backend.status = deviceStatus{rate: 4000, low: 1000, channels: 15}
	for _, v := range spectrumSamples(12000, 4000, 125) {
		s.backend.history.add(v)
	}
	u := ui.NewTester(s.view, 1280, 900)
	if err := u.Click("频谱"); err != nil {
		t.Fatal(err)
	}
	if s.plotMode != 1 || !u.HasText("峰值 125.00 Hz") {
		t.Fatal("frequency tab or peak bubble missing", u.Texts())
	}
	for _, name := range []string{"R · 红通道", "G · 绿通道", "B · 蓝通道", "Clear · 透明通道"} {
		plot, ok := u.Find(name + "频谱")
		if !ok || plot.W < 400 || plot.H < 60 {
			t.Fatal("spectrum geometry", plot)
		}
		peak, ok := u.Find(name + "峰值频率")
		if !ok || peak.X < 0 || peak.X+peak.W > 1280 || peak.Y < 0 || peak.Y+peak.H > 900 {
			t.Fatal("peak bubble outside window", peak)
		}
	}
	saveScreen(t, "spectrum-desktop", u)
	u.SetSize(1080, 760)
	saveScreen(t, "spectrum-minimum", u)
	if err := u.Click("暂停刷新"); err != nil {
		t.Fatal(err)
	}
	end := s.end
	s.backend.history.add(sample{sequence: 12000, micros: 3000000, channels: 15})
	u.Frame()
	if !s.paused || s.end != end || !u.HasText("峰值 125.00 Hz") {
		t.Fatal("paused FFT moved with acquisition")
	}
	if err := u.Click("数值波形"); err != nil {
		t.Fatal(err)
	}
	if s.plotMode != 0 || !s.paused {
		t.Fatal("switching mode discarded snapshot")
	}
	if _, ok := u.Find("R · 红通道频谱"); ok {
		t.Fatal("spectrum remains in waveform mode")
	}
	u.Key(0, ui.KeyEscape)
	if s.paused {
		t.Fatal("Escape did not resume")
	}
}
