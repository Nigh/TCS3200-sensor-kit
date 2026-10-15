package main

import (
	"encoding/binary"
	"testing"
)

func TestSamplingSchedule(t *testing.T) {
	for _, tt := range []struct {
		low, slot uint32
		rate      uint16
	}{
		{4000, 1040, 240}, {5000, 840, 297}, {10000, 440, 568}, {15000, 307, 814}, {19047, 251, 996}, {19048, 250, 1000}, {20000, 250, 1000}, {600000, 250, 1000},
	} {
		if slot, rate := channelSlotUS(tt.low), maximumRateHz(tt.low); slot != tt.slot || rate != tt.rate {
			t.Fatalf("%d: slot=%d rate=%d", tt.low, slot, rate)
		}
		p := make([]byte, 32)
		binary.LittleEndian.PutUint16(p, tt.rate)
		binary.LittleEndian.PutUint16(p[2:], uint16(tt.slot))
		binary.LittleEndian.PutUint32(p[4:], tt.low)
		if _, ok := decodeStatus(p); !ok {
			t.Fatalf("valid status rejected: %v", tt)
		}
		p[2] ^= 1
		if _, ok := decodeStatus(p); ok {
			t.Fatal("incoherent status accepted")
		}
	}
}
func TestClampLowInput(t *testing.T) {
	for _, tt := range []struct {
		text string
		want uint32
	}{
		{"1", 4000}, {"-20", 4000}, {"4000", 4000}, {"600000", 600000}, {"600001", 600000}, {"1e99", 600000}, {"1e999", 600000}, {"-1e999", 4000}, {"4200.6", 4201}, {" 5000 ", 5000}, {"", 15000}, {"invalid", 15000}, {"NaN", 15000},
	} {
		if got := clampedLowInput(tt.text, 15000); got != tt.want {
			t.Fatalf("%q got %d want %d", tt.text, got, tt.want)
		}
	}
}
