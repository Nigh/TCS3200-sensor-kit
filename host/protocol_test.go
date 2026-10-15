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
	vectors := [][]byte{unhex("54430101010000005597"), unhex("5443010202000100005cb1"), unhex("544301800200020002005edd"), unhex("5443010603000400983a0000185b"), unhex("544301400000160000000000a086010000000000000000000f0000000000d49e")}
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
	d.feed(unhex("544301400000160000000000a086010000000000000000000f0000000000d49e"), func(f frame) {
		s, ok := decodeSample(f)
		if !ok || s.micros != 100000 || s.low != 15 {
			t.Fatal(s, ok)
		}
	})
}
