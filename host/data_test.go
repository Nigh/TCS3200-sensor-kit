package main

import (
	"math"
	"slices"
	"testing"
)

func TestHistory(t *testing.T) {
	var h history
	for i := range 20000 {
		h.add(sample{channels: 15, sequence: uint32(i), micros: uint64(uint32(0xffff0000 + uint64(i)*1000))})
	}
	data := h.copyTo(nil)
	if len(data) != 10001 || data[len(data)-1].micros-data[0].micros != windowUS {
		t.Fatal(len(data))
	}
	snapshot := slices.Clone(data)
	next := uint64(0xffff0000 + 20002*1000)
	h.add(sample{channels: 15, sequence: 20002, micros: uint64(uint32(next))})
	if h.drops != 2 || !slices.Equal(snapshot, data) {
		t.Fatal("drop/freeze")
	}
	if h.add(sample{channels: 15, sequence: 20002, micros: h.elapsed}) {
		t.Fatal("duplicate")
	}
	h.add(sample{channels: 15, sequence: 1, micros: 1000})
	if h.size != 1 {
		t.Fatal("reset")
	}
}
func TestMinmaxAndStats(t *testing.T) {
	xs := []float32{0, .01, .02, .03, .4, .5, .6, 1}
	ys := []float32{0, 10, -5, 2, 3, float32(math.NaN()), 4, 5}
	x, y := minmax(nil, nil, xs, ys, 2)
	if !slices.Contains(y, float32(10)) || !slices.Contains(y, float32(-5)) || x[0] != 0 || x[len(x)-1] != 1 {
		t.Fatal(x, y)
	}
	gap := false
	for _, v := range y {
		gap = gap || math.IsNaN(float64(v))
	}
	if !gap {
		t.Fatal("gap lost")
	}
	data := []sample{{channels: 15, micros: 1, values: [4]uint16{100}}, {channels: 15, micros: 2, values: [4]uint16{400}}, {channels: 15, micros: 3, values: [4]uint16{200}}, {channels: 15, micros: 4, values: [4]uint16{300}}}
	s, _ := bucketStats(data, 0, 1, 5, nil)
	if s.count != 4 || s.min != 125 || s.max != 500 || math.Abs(s.median-208.33333333333334) > 1e-9 {
		t.Fatal(s)
	}
	s, _ = bucketStats(data, 0, 1, 4, nil)
	if s.median != 250 {
		t.Fatal(s)
	}
	s, _ = bucketStats(data, 0, 5, 6, nil)
	if s.count != 0 {
		t.Fatal(s)
	}
}

func TestThreeMinutesOfSamplesStayBounded(t *testing.T) {
	var h history
	for i := range 180000 {
		h.add(sample{channels: 15, sequence: uint32(i), micros: uint64(i) * 1000})
	}
	if h.size != 10001 || h.drops != 0 {
		t.Fatal(h.size, h.drops)
	}
	data := h.copyTo(nil)
	if data[0].micros != 169999000 || data[len(data)-1].micros != 179999000 {
		t.Fatal("retention window")
	}
}
func BenchmarkMinMax(b *testing.B) {
	xs, ys := make([]float32, 10001), make([]float32, 10001)
	for i := range xs {
		xs[i] = float32(i) / 10000
		ys[i] = float32(math.Sin(float64(i) * .07))
	}
	var x, y []float32
	x, y = minmax(x, y, xs, ys, 1000)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		x, y = minmax(x, y, xs, ys, 1000)
	}
}

func TestSequenceWrap(t *testing.T) {
	var h history
	h.add(sample{channels: 15, sequence: 0xfffffffe, micros: 1000})
	h.add(sample{channels: 15, sequence: 0xffffffff, micros: 2000})
	h.add(sample{channels: 15, sequence: 0, micros: 3000})
	if h.size != 3 || h.drops != 0 {
		t.Fatal(h.size, h.drops)
	}
}

func TestSampleDetail(t *testing.T) {
	data := []sample{
		{micros: 0, channels: 1},
		{micros: 1000, channels: 1},
		{micros: 2000, channels: 1},
		{micros: 3000, channels: 1},
	}
	if raw, nearest := sampleDetail(data, 0, 0, 3000, 4, 1600); !raw || nearest != 2 {
		t.Fatal(raw, nearest)
	}
	if raw, _ := sampleDetail(data, 0, 0, 3000, 2, 1600); raw {
		t.Fatal("colliding pixel bins require aggregation")
	}
	if raw, _ := sampleDetail(data, 0, 0, 3000, 0, 1600); raw {
		t.Fatal("unmeasured width")
	}
	if raw, _ := sampleDetail(data, 1, 0, 3000, 4, 1600); raw {
		t.Fatal("disabled channel")
	}
	if raw, nearest := sampleDetail(data, 0, 2000, 3000, 4, 3000); !raw || nearest != 3 {
		t.Fatal("right endpoint", raw, nearest)
	}
	data[1].micros = 100
	if raw, _ := sampleDetail(data, 0, 0, 3000, 4, 100); raw {
		t.Fatal("irregular cadence collision")
	}
	if raw, _ := sampleDetail(data, 0, 4000, 5000, 4, 4500); raw {
		t.Fatal("empty window")
	}
}

func TestPulseWidths(t *testing.T) {
	if pulseUS(10000) != 5 || pulseText(1234) != "40.5 μs" || pulseText(0) != "— μs" || !math.IsNaN(pulseUS(65535)) {
		t.Fatal("pulse conversion or missing reading")
	}
	data := []sample{
		{channels: 1, values: [4]uint16{0}},
		{channels: 1, values: [4]uint16{65535}},
		{channels: 1, values: [4]uint16{10000}},
		{channels: 1, values: [4]uint16{20000}},
	}
	stats, _ := bucketStats(data, 0, 0, 1, nil)
	mean, low, high, n := recentStats(data, 0)
	if stats.count != 2 || stats.median != 3.75 || mean != 3.75 || low != 2.5 || high != 5 || n != 2 {
		t.Fatal(stats, mean, low, high, n)
	}
}
