package main

import (
	"math"
	"math/cmplx"
	"sort"
)

const maxFFT = 4096
const minFFT = 32

type spectrumKey struct {
	epoch, first, last uint64
	sequence           uint32
	count              int
	channel            int
}

type spectrum struct {
	key                                  spectrumKey
	cached                               bool
	work                                 []complex128
	window, amplitudes, noise            []float64
	weightSum                            float64
	n                                    int
	rate, maximum, peakHz, peakAmplitude float64
	hasPeak                              bool
	message                              string
	points                               [][2]float32
}

// update uses the latest contiguous valid suffix inside the inspected viewport.
// The cap bounds both FFT work and memory independently of acquisition rate.
func (s *spectrum) update(data []sample, ch int, lo, hi float64, epoch uint64) {
	end := sort.Search(len(data), func(i int) bool { return float64(data[i].micros) > hi })
	start := sort.Search(end, func(i int) bool { return float64(data[i].micros) >= lo })
	start = max(start, end-maxFFT)
	if start == end {
		s.cached = false
		s.n, s.hasPeak = 0, false
		s.message = "等待连续有效采样"
		return
	}
	key := spectrumKey{epoch: epoch, first: data[start].micros, last: data[end-1].micros, sequence: data[end-1].sequence, count: end - start, channel: ch}
	if s.cached && s.key == key {
		return
	}
	s.key, s.cached = key, true
	s.n, s.hasPeak = 0, false
	s.message = "连续有效采样不足"
	from := end
	for from > start {
		v := data[from-1]
		if v.channels&(1<<ch) == 0 || v.low&(1<<ch) != 0 || v.clip&(1<<ch) != 0 || v.flags != 0 || math.IsNaN(pulseUS(v.values[ch])) {
			break
		}
		if from < end {
			next := data[from]
			if next.sequence-v.sequence != 1 || next.micros <= v.micros {
				break
			}
		}
		from--
	}
	n := 1
	for n*2 <= end-from {
		n *= 2
	}
	if n < minFFT {
		return
	}
	from = end - n
	dt := float64(data[end-1].micros-data[from].micros) / float64(n-1)
	for i := from + 1; i < end; i++ {
		if math.Abs(float64(data[i].micros-data[i-1].micros)-dt) > max(2, dt*.1) {
			s.message = "采样间隔不均匀，暂不计算频谱"
			return
		}
	}
	s.n, s.rate, s.message = n, 1e6/dt, ""
	if cap(s.work) < n {
		s.work = make([]complex128, n)
	}
	s.work = s.work[:n]
	if len(s.window) != n {
		if cap(s.window) < n {
			s.window = make([]float64, n)
		}
		s.window = s.window[:n]
		s.weightSum = 0
		for i := range n {
			s.window[i] = .5 - .5*math.Cos(2*math.Pi*float64(i)/float64(n-1))
			s.weightSum += s.window[i]
		}
	}
	mean := 0.0
	for i := range n {
		value := pulseUS(data[from+i].values[ch])
		s.work[i] = complex(value, 0)
		mean += value * s.window[i]
	}
	mean /= s.weightSum
	for i := range n {
		s.work[i] = complex((real(s.work[i])-mean)*s.window[i], 0)
	}
	fft(s.work)
	bins := n/2 + 1
	if cap(s.amplitudes) < bins {
		s.amplitudes = make([]float64, bins)
		s.noise = make([]float64, bins)
	}
	s.amplitudes, s.noise = s.amplitudes[:bins], s.noise[:bins-1]
	s.amplitudes[0], s.maximum = 0, 0
	peak := 1
	for i := 1; i < bins; i++ {
		amplitude := 2 * cmplx.Abs(s.work[i]) / s.weightSum
		if i == n/2 {
			amplitude /= 2
		}
		s.amplitudes[i], s.noise[i-1] = amplitude, amplitude
		if amplitude > s.maximum {
			s.maximum, peak = amplitude, i
		}
	}
	sort.Float64s(s.noise)
	floor := s.noise[len(s.noise)/2]
	// ponytail: annotate only the dominant peak above 8× median noise;
	// use a configurable multi-peak detector if harmonic analysis is needed.
	s.hasPeak = s.maximum > max(1e-6, floor*8)
	offset := 0.0
	if peak > 1 && peak < n/2 {
		a, b, c := math.Log(s.amplitudes[peak-1]), math.Log(s.amplitudes[peak]), math.Log(s.amplitudes[peak+1])
		if denominator := a - 2*b + c; !math.IsInf(denominator, 0) && !math.IsNaN(denominator) && denominator != 0 {
			offset = max(-.5, min(.5, .5*(a-c)/denominator))
		}
	}
	s.peakHz = (float64(peak) + offset) * s.rate / float64(n)
	s.peakAmplitude = s.maximum
}

// fft is an in-place radix-2 transform; callers supply a power-of-two length.
func fft(values []complex128) {
	n := len(values)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for j&bit != 0 {
			j ^= bit
			bit >>= 1
		}
		j ^= bit
		if i < j {
			values[i], values[j] = values[j], values[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		angle := -2 * math.Pi / float64(size)
		root := complex(math.Cos(angle), math.Sin(angle))
		for start := 0; start < n; start += size {
			factor := complex(1, 0)
			for i := range size / 2 {
				a := values[start+i]
				b := factor * values[start+i+size/2]
				values[start+i], values[start+i+size/2] = a+b, a-b
				factor *= root
			}
		}
	}
}
