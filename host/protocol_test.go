package main

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func unhex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}
func TestProtocolVectors(t *testing.T) {
	if crc16([]byte("123456789")) != 0x29b1 {
		t.Fatal("CRC")
	}
	vectors := [][]byte{unhex("5443030101000000151c"), unhex("544303020200010000bfd1"), unhex("54430380020002000200f852"), unhex("5443030603000400983a00009285"), unhex("544303400000160000000000a086010000000000000000000f000000000f1326")}
	for _, v := range vectors {
		for split := 0; split <= len(v); split++ {
			var d decoder
			var got []frame
			accept := func(f frame) { got = append(got, f) }
			d.feed(v[:split], accept)
			d.feed(v[split:], accept)
			if len(got) != 1 || !bytes.Equal(encode(got[0].kind, got[0].sequence, got[0].payload), v) {
				t.Fatalf("split %d: %x", split, v)
			}
		}
	}
	var d decoder
	n := 0
	accept := func(frame) { n++ }
	bad := bytes.Clone(vectors[0])
	bad[8] ^= 1
	d.feed(append(append(append([]byte{1, 0x54}, bad...), vectors[0]...), vectors[1]...), accept)
	if n != 2 || d.errors != 1 {
		t.Fatalf("resync %d %d", n, d.errors)
	}
	d.feed(vectors[0][:5], accept)
	d.clear()
	d.feed(vectors[0], accept)
	if n != 3 {
		t.Fatal("truncated recovery")
	}
}
func TestSampleVector(t *testing.T) {
	var d decoder
	d.feed(unhex("544303400000160000000000a086010000000000000000000f000000000f1326"), func(f frame) {
		s, ok := decodeSample(f)
		if !ok || s.micros != 100000 || s.low != 15 {
			t.Fatal(s, ok)
		}
	})
}

func TestInvalidHeaderAndMalformedSample(t *testing.T) {
	var d decoder
	n := 0
	garbage := []byte{0x54, 0x43, 1, 1, 0, 0, 0, 0, 0x54, 0x43, 2, 1, 0, 0, 65, 0}
	d.feed(append(garbage, encode(cmdStatus, 1, nil)...), func(frame) { n++ })
	if n != 1 || d.errors != 2 {
		t.Fatal(n, d.errors)
	}
	if _, ok := decodeSample(frame{kind: msgSample, payload: make([]byte, 21)}); ok {
		t.Fatal("short sample")
	}
	if _, ok := decodeSample(frame{kind: msgSample, sequence: 1, payload: make([]byte, 22)}); ok {
		t.Fatal("sequence mismatch")
	}
}

func TestUSBPollStatus(t *testing.T) {
	for _, hz := range []uint16{0, 125, 500, 1000, 65535} {
		p := []byte{0xe8, 3, byte(hz), byte(hz >> 8)}
		s, ok := decodeUSBStatus(p)
		if !ok || s.idlePollHz != hz || s.intervalUS != 1000 {
			t.Fatal(s, ok)
		}
	}
	if _, ok := decodeUSBStatus([]byte{1, 0, 0, 0}); ok {
		t.Fatal("invalid interval")
	}
	if _, ok := decodeUSBStatus([]byte{0}); ok {
		t.Fatal("short status")
	}
	if (usbStatus{1000, 65535}).text() != "HID 请求 1000 Hz · 空闲轮询估算：暂不可测" {
		t.Fatal("unavailable status")
	}
}
