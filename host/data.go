package main

import (
	"math"
	"slices"
)

const capacity = 40001
const windowUS = uint64(10_000_000)

type history struct {
	data                  [capacity]sample
	head, size            int
	lastRaw, lastSequence uint32
	elapsed               uint64
	started               bool
	drops                 uint64
}

func (h *history) add(s sample) bool {
	raw := uint32(s.micros)
	if h.started {
		dt, ds := raw-h.lastRaw, s.sequence-h.lastSequence
		if ds == 0 {
			return false
		}
		// A backwards sequence or clock is a new device session, not a 71-minute wrap.
		if ds > 1<<31 || dt > 1<<31 {
			h.head, h.size, h.elapsed, h.drops, h.started = 0, 0, 0, 0, false
		} else {
			h.elapsed += uint64(dt)
			h.drops += uint64(ds - 1)
		}
	}
	if !h.started {
		h.elapsed = uint64(raw)
		h.started = true
	}
	h.lastRaw, h.lastSequence = raw, s.sequence
	s.micros = h.elapsed
	for h.size > 0 && s.micros-h.data[h.head].micros > windowUS {
		h.head = (h.head + 1) % capacity
		h.size--
	}
	if h.size == capacity {
		h.head = (h.head + 1) % capacity
		h.size--
	}
	h.data[(h.head+h.size)%capacity] = s
	h.size++
	return true
}
func (h *history) copyTo(dst []sample) []sample {
	dst = dst[:0]
	for i := range h.size {
		dst = append(dst, h.data[(h.head+i)%capacity])
	}
	return dst
}

type statistics struct {
	min, max, median float64
	count            int
}

func bucketStats(data []sample, ch int, lo, hi float64, scratch []uint16) (statistics, []uint16) {
	scratch = scratch[:0]
	for _, s := range data {
		if s.channels&(1<<ch) != 0 && float64(s.micros) >= lo && float64(s.micros) < hi {
			scratch = append(scratch, s.values[ch])
		}
	}
	if len(scratch) == 0 {
		return statistics{}, scratch
	}
	slices.Sort(scratch)
	n := len(scratch)
	med := float64(scratch[n/2])
	if n%2 == 0 {
		med = (med + float64(scratch[n/2-1])) / 2
	}
	return statistics{float64(scratch[0]) / 100, float64(scratch[n-1]) / 100, med / 100, n}, scratch
}

// shortcut: app-side pixel Min/Max uses the existing CPU path; replace with upstream chart.MinMax and StrokePolyline after mygo PR #363 lands.
func minmax(dstX, dstY, xs, ys []float32, width int) ([]float32, []float32) {
	dstX, dstY = dstX[:0], dstY[:0]
	if width <= 0 {
		return dstX, dstY
	}
	emit := func(i int) { dstX = append(dstX, xs[i]); dstY = append(dstY, ys[i]) }
	for start := 0; start < len(xs); {
		if math.IsNaN(float64(ys[start])) {
			emit(start)
			start++
			continue
		}
		end := start + 1
		for end < len(xs) && !math.IsNaN(float64(ys[end])) {
			end++
		}
		last := -1
		add := func(i int) {
			if i != last {
				emit(i)
				last = i
			}
		}
		add(start)
		for i := start; i < end; {
			bucket := int(math.Floor(float64(xs[i]) * float64(width)))
			low, high := i, i
			i++
			for i < end && int(math.Floor(float64(xs[i])*float64(width))) == bucket {
				if ys[i] < ys[low] {
					low = i
				}
				if ys[i] > ys[high] {
					high = i
				}
				i++
			}
			add(min(low, high))
			add(max(low, high))
		}
		add(end - 1)
		start = end
	}
	return dstX, dstY
}

// Recent statistics use only valid measurements of enabled channels.
func recentStats(data []sample, ch int) (mean, low, high float64, count int) {
	if len(data) == 0 {
		return
	}
	end := data[len(data)-1].micros
	for _, s := range data {
		if end-s.micros > 500000 || s.channels&(1<<ch) == 0 || s.clip&(1<<ch) != 0 || s.flags != 0 {
			continue
		}
		v := float64(s.values[ch]) / 100
		if count == 0 {
			low, high = v, v
		}
		low, high = min(low, v), max(high, v)
		mean += v
		count++
	}
	if count > 0 {
		mean /= float64(count)
	}
	return
}

// Individual samples need no aggregation when every occupied physical pixel has one sample.
func sampleDetail(data []sample, ch int, lo, hi float64, pixels int, hover float64) (raw bool, nearest int) {
	nearest, previousBin := -1, -1
	if pixels <= 0 || hi <= lo {
		return false, -1
	}
	distance := math.Inf(1)
	for i, s := range data {
		t := float64(s.micros)
		if s.channels&(1<<ch) == 0 || t < lo || t > hi {
			continue
		}
		bin := min(pixels-1, int(math.Floor((t-lo)/(hi-lo)*float64(pixels))))
		if bin == previousBin {
			return false, -1
		}
		previousBin = bin
		if d := math.Abs(t - hover); d < distance {
			distance, nearest = d, i
		}
	}
	return nearest >= 0, nearest
}
