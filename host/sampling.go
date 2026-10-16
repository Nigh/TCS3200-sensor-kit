package main

import (
	"math"
	"strconv"
	"strings"
)

const (
	lowMinHz        = uint32(4000)
	lowMaxHz        = uint32(600000)
	defaultLowHz    = uint32(15000)
	protocolVersion = byte(3)
)

// Keep the timing budget in sync with firmware/src/protocol.c and protocol v3.
func channelSlotUS(low uint32) uint32 {
	low = max(lowMinHz, min(lowMaxHz, low))
	return max(250, (4_000_000+low-1)/low+40)
}
func maximumRateHz(low uint32) uint16 { return uint16(1_000_000 / (4 * channelSlotUS(low))) }

// Normalize on blur/submit, allowing partial numeric edits while typing.
func clampedLowInput(text string, fallback uint32) uint32 {
	if fallback < lowMinHz || fallback > lowMaxHz {
		fallback = defaultLowHz
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if math.IsNaN(value) || (err != nil && !math.IsInf(value, 0)) {
		return fallback
	}
	return uint32(math.Round(max(float64(lowMinHz), min(float64(lowMaxHz), value))))
}

func maskedRateHz(low uint32, mask byte) uint16 {
	count := uint32(0)
	for ch := range 4 {
		count += uint32(mask >> ch & 1)
	}
	if count == 0 {
		return 0
	}
	return uint16(1_000_000 / (count * channelSlotUS(low)))
}
