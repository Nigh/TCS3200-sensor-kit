package main

import (
	"math"
	"slices"
	"testing"
)

func TestHistory(t *testing.T) {
	var h history
	for i := range 20000 {
		h.add(sample{sequence: uint32(i), micros: uint64(uint32(0xffff0000 + uint64(i)*1000))})
	}
	data := h.copyTo(nil)
	if len(data) != 10001 || data[len(data)-1].micros-data[0].micros != windowUS {
		t.Fatal(len(data))
	}
	snapshot := slices.Clone(data)
	next := uint64(0xffff0000 + 20002*1000)
	h.add(sample{sequence: 20002, micros: uint64(uint32(next))})
	if h.drops != 2 || !slices.Equal(snapshot, data) {
		t.Fatal("drop/freeze")
	}
	if h.add(sample{sequence: 20002, micros: h.elapsed}) {
		t.Fatal("duplicate")
	}
	h.add(sample{sequence: 1, micros: 1000})
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
	data := []sample{{micros: 1, values: [4]uint16{100}}, {micros: 2, values: [4]uint16{400}}, {micros: 3, values: [4]uint16{200}}, {micros: 4, values: [4]uint16{300}}}
	s, _ := bucketStats(data, 0, 1, 5, nil)
	if s.count != 4 || s.min != 1 || s.max != 4 || s.median != 2.5 {
		t.Fatal(s)
	}
	s, _ = bucketStats(data, 0, 1, 4, nil)
	if s.median != 2 {
		t.Fatal(s)
	}
	s, _ = bucketStats(data, 0, 5, 6, nil)
	if s.count != 0 {
		t.Fatal(s)
	}
}
