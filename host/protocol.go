package main

import "encoding/binary"

const (
	cmdStatus = 0x01
	cmdLow    = 0x06
	msgSample = 0x40
	msgACK    = 0x80
	msgStatus = 0x81
)

type frame struct {
	kind     byte
	sequence uint16
	payload  []byte
}

func crc16(data []byte) uint16 {
	crc := uint16(0xffff)
	for _, b := range data {
		crc ^= uint16(b) << 8
		for range 8 {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
func encode(kind byte, sequence uint16, payload []byte) []byte {
	b := make([]byte, 10+len(payload))
	b[0], b[1], b[2], b[3] = 0x54, 0x43, protocolVersion, kind
	binary.LittleEndian.PutUint16(b[4:], sequence)
	binary.LittleEndian.PutUint16(b[6:], uint16(len(payload)))
	copy(b[8:], payload)
	binary.LittleEndian.PutUint16(b[8+len(payload):], crc16(b[2:8+len(payload)]))
	return b
}

type decoder struct {
	buf    []byte
	errors uint64
}

func (d *decoder) clear() {
	if len(d.buf) > 0 {
		d.errors++
	}
	d.buf = d.buf[:0]
}
func (d *decoder) feed(data []byte, accept func(frame)) {
	for _, b := range data {
		d.buf = append(d.buf, b)
		for len(d.buf) > 0 {
			if d.buf[0] != 0x54 || len(d.buf) > 1 && d.buf[1] != 0x43 {
				d.buf = d.buf[1:]
				continue
			}
			if len(d.buf) < 8 {
				break
			}
			n := int(binary.LittleEndian.Uint16(d.buf[6:]))
			if d.buf[2] != protocolVersion || n > 64 {
				d.errors++
				d.buf = d.buf[1:]
				continue
			}
			if len(d.buf) < n+10 {
				break
			}
			if crc16(d.buf[2:8+n]) != binary.LittleEndian.Uint16(d.buf[8+n:]) {
				d.errors++
				d.buf = d.buf[1:]
				continue
			}
			accept(frame{d.buf[3], binary.LittleEndian.Uint16(d.buf[4:]), append([]byte(nil), d.buf[8:8+n]...)})
			d.buf = d.buf[n+10:]
		}
	}
}

type sample struct {
	sequence                      uint32
	micros                        uint64
	values                        [4]uint16
	low, clip, mode, state, flags byte
}

func decodeSample(f frame) (sample, bool) {
	if len(f.payload) != 22 {
		return sample{}, false
	}
	p := f.payload
	s := sample{sequence: binary.LittleEndian.Uint32(p), micros: uint64(binary.LittleEndian.Uint32(p[4:])), low: p[16], clip: p[17], mode: p[18], state: p[19], flags: p[20]}
	for i := range 4 {
		s.values[i] = binary.LittleEndian.Uint16(p[8+i*2:])
	}
	return s, f.sequence == uint16(s.sequence) && s.mode <= 1 && s.state <= 5 && s.low&0xf0 == 0 && s.clip&0xf0 == 0 && s.flags&^byte(1) == 0 && p[21] == 0
}

type deviceStatus struct {
	rate, slot                                                    uint16
	low, capture, captureDrops, streamDrops, parserErrors, uptime uint32
	mode, state, configured                                       byte
}

func decodeStatus(p []byte) (deviceStatus, bool) {
	if len(p) != 32 {
		return deviceStatus{}, false
	}
	s := deviceStatus{rate: binary.LittleEndian.Uint16(p), slot: binary.LittleEndian.Uint16(p[2:]), mode: p[28], state: p[29], configured: p[30]}
	fields := []*uint32{&s.low, &s.capture, &s.captureDrops, &s.streamDrops, &s.parserErrors, &s.uptime}
	for i, dst := range fields {
		*dst = binary.LittleEndian.Uint32(p[4+4*i:])
	}
	return s, s.low >= lowMinHz && s.low <= lowMaxHz && s.rate == maximumRateHz(s.low) && uint32(s.slot) == channelSlotUS(s.low) && s.mode <= 1 && s.state <= 5 && s.configured <= 1 && p[31] == 0
}
