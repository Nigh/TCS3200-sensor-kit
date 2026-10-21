package main

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"time"
)

var levels = [5]int{0, 64, 128, 192, 255}
var permutations = [6][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}

type calibrationNode struct {
	RGB   [3]int
	Hz    [4]float64
	Pulse [4]*float64
	Count [4]int
	Low   byte
}
type calibrationMap struct {
	Serial  string
	Created time.Time
	LowHz   uint32
	Nodes   []calibrationNode
}

func targetRGB(i int) [3]int { return [3]int{levels[i/25], levels[i/5%5], levels[i%5]} }
func (m *calibrationMap) tetrahedra(visit func([4]calibrationNode) bool) bool {
	if m == nil || len(m.Nodes) != 125 {
		return false
	}
	for r := 0; r < 4; r++ {
		for g := 0; g < 4; g++ {
			for b := 0; b < 4; b++ {
				for _, order := range permutations {
					index := [3]int{r, g, b}
					var nodes [4]calibrationNode
					nodes[0] = m.Nodes[index[0]*25+index[1]*5+index[2]]
					for j, axis := range order {
						index[axis]++
						nodes[j+1] = m.Nodes[index[0]*25+index[1]*5+index[2]]
					}
					if visit(nodes) {
						return true
					}
				}
			}
		}
	}
	return false
}
func determinant(a, b, c [3]float64) float64 {
	return a[0]*(b[1]*c[2]-b[2]*c[1]) - b[0]*(a[1]*c[2]-a[2]*c[1]) + c[0]*(a[1]*b[2]-a[2]*b[1])
}
func barycentric(nodes [4]calibrationNode, hz [3]float64) ([4]float64, bool) {
	var edges [3][3]float64
	var delta [3]float64
	for c := range 3 {
		delta[c] = hz[c] - nodes[0].Hz[c]
		for j := range 3 {
			edges[j][c] = nodes[j+1].Hz[c] - nodes[0].Hz[c]
		}
	}
	d := determinant(edges[0], edges[1], edges[2])
	scale := 1.0
	for _, e := range edges {
		scale *= math.Sqrt(e[0]*e[0] + e[1]*e[1] + e[2]*e[2])
	}
	if scale <= 0 || math.Abs(d) <= 1e-8*scale {
		return [4]float64{}, false
	}
	w := [4]float64{0, determinant(delta, edges[1], edges[2]) / d, determinant(edges[0], delta, edges[2]) / d, determinant(edges[0], edges[1], delta) / d}
	w[0] = 1 - w[1] - w[2] - w[3]
	for _, v := range w {
		if v < -1e-7 || v > 1+1e-7 {
			return w, false
		}
	}
	return w, true
}
func (m *calibrationMap) inverse(hz [3]float64) (rgb [3]int, ok bool) {
	m.tetrahedra(func(nodes [4]calibrationNode) bool {
		for _, n := range nodes {
			if n.Low&7 != 0 {
				return false
			}
		}
		w, inside := barycentric(nodes, hz)
		if !inside {
			return false
		}
		for ch := range 3 {
			v := 0.0
			for j, n := range nodes {
				v += w[j] * float64(n.RGB[ch])
			}
			rgb[ch] = int(math.Round(max(0, min(255, v))))
		}
		ok = true
		return true
	})
	return
}

// The same tetrahedral interpolation is used in both directions.
func (m *calibrationMap) forward(rgb [3]int) (hz [4]float64, ok bool) {
	if m == nil {
		return
	}
	m.tetrahedra(func(nodes [4]calibrationNode) bool {
		space := nodes
		for i := range space {
			for c := range 3 {
				space[i].Hz[c] = float64(space[i].RGB[c])
			}
		}
		w, inside := barycentric(space, [3]float64{float64(rgb[0]), float64(rgb[1]), float64(rgb[2])})
		if !inside {
			return false
		}
		for _, n := range nodes {
			if n.Low != 0 {
				return false
			}
		}
		for c := range 4 {
			for j, n := range nodes {
				hz[c] += w[j] * n.Hz[c]
			}
		}
		ok = true
		return true
	})
	return
}
func (m *calibrationMap) validate() error {
	if m == nil || len(m.Nodes) != 125 || m.LowHz != 100 {
		return errors.New("校准映射不完整")
	}
	for i, n := range m.Nodes {
		if n.RGB != targetRGB(i) || n.Low > 15 {
			return errors.New("校准节点格式错误")
		}
		for c, hz := range n.Hz {
			if math.IsNaN(hz) || math.IsInf(hz, 0) || hz < float64(m.LowHz) || hz > 655340 || n.Count[c] < 6 {
				return errors.New("校准节点数据无效")
			}
			if n.Low&(1<<c) == 0 && (n.Pulse[c] == nil || math.Abs(*n.Pulse[c]-500000/hz) > 0.001) {
				return errors.New("校准脉宽无效")
			}
		}
	}
	for c := range 4 {
		if m.Nodes[124].Low&(1<<c) != 0 || m.Nodes[124].Hz[c]-m.Nodes[0].Hz[c] < max(10, m.Nodes[124].Hz[c]*0.01) {
			return errors.New("黑白响应不可区分，请检查传感器位置和屏幕亮度")
		}
	}
	valid := m.tetrahedra(func(n [4]calibrationNode) bool {
		for _, v := range n {
			if v.Low&7 != 0 {
				return false
			}
		}
		_, ok := barycentric(n, [3]float64{n[0].Hz[0], n[0].Hz[1], n[0].Hz[2]})
		return ok
	})
	if !valid {
		return errors.New("RGB 响应退化，无法反解颜色")
	}
	return nil
}
func (m *calibrationMap) response(hz uint32, ch int) float64 {
	black, white := m.Nodes[0].Hz[ch], m.Nodes[124].Hz[ch]
	return max(0, min(1, (float64(hz)-black)/(white-black)))
}

const calibrationSettle = 100 * time.Millisecond

type calibrationRun struct {
	RestoreRequested bool
	ColorStart       time.Time
	Map              calibrationMap
	OldLow           uint32
	OldChannels      byte
	Stage            string
	Painted          time.Time
	After            uint64
	LastSequence     uint32
	Seen             bool
	Samples          [4][]float64
	Low              byte
}

func (r *calibrationRun) collect(data []sample, now time.Time) (bool, error) {
	if !r.ColorStart.IsZero() && now.Sub(r.ColorStart) > 5*time.Second {
		return false, errors.New("颜色绘制或采样超时，请重新校准")
	}
	if r.Painted.IsZero() || now.Sub(r.Painted) < calibrationSettle {
		return false, nil
	}
	if now.Sub(r.Painted) > 5*time.Second {
		return false, errors.New("当前颜色超过 5 秒没有足够有效采样，请检查信号或重新校准")
	}
	for _, v := range data {
		if v.micros < r.After || r.Seen && int32(v.sequence-r.LastSequence) <= 0 {
			continue
		}
		continuous := !r.Seen || v.sequence-r.LastSequence == 1
		r.LastSequence = v.sequence
		r.Seen = true
		if v.channels != 15 || v.mode != 0 || v.state != 0 || v.flags != 0 || v.clip != 0 {
			r.Samples = [4][]float64{}
			continue
		}
		if !continuous || r.Low != v.low {
			r.Samples = [4][]float64{}
		}
		r.Low = v.low
		for c := range 4 {
			hz := float64(v.values[c])
			if v.low&(1<<c) != 0 {
				hz = 100
				r.Low |= 1 << c
			}
			if hz < 100 || hz > 655340 {
				return false, errors.New("校准频率无效")
			}
			r.Samples[c] = append(r.Samples[c], hz)
			if len(r.Samples[c]) > 6 {
				r.Samples[c] = r.Samples[c][1:]
			}
		}
		if len(r.Samples[0]) == 6 {
			stable := true
			for c := range 4 {
				values := r.Samples[c]
				lo, hi := slices.Min(values), slices.Max(values)
				if hi-lo > max(2, (hi+lo)*0.01) {
					stable = false
				}
			}
			if stable {
				return r.completeColor(now), nil
			}
		}
	}
	return false, nil
}

func (r *calibrationRun) completeColor(now time.Time) bool {
	n := calibrationNode{RGB: targetRGB(len(r.Map.Nodes)), Low: r.Low}
	for c := range 4 {
		values := r.Samples[c]
		slices.Sort(values)
		mid := len(values) / 2
		hz := values[mid]
		if len(values)%2 == 0 {
			hz = (hz + values[mid-1]) / 2
		}
		n.Hz[c] = hz
		n.Count[c] = len(values)
		if n.Low&(1<<c) == 0 {
			p := 500000 / hz
			n.Pulse[c] = &p
		}
	}
	r.Map.Nodes = append(r.Map.Nodes, n)
	r.Samples = [4][]float64{}
	r.Low = 0
	r.Painted = time.Time{}
	r.ColorStart = now
	r.Seen = false
	return true
}
func colorText(rgb [3]int, format int) string {
	r, g, b := rgb[0], rgb[1], rgb[2]
	switch format {
	case 1:
		return fmt.Sprintf("#%02X%02X%02X", r, g, b)
	case 2:
		rr, gg, bb := float64(r)/255, float64(g)/255, float64(b)/255
		hi, lo := max(rr, gg, bb), min(rr, gg, bb)
		l := (hi + lo) / 2
		h, s := 0.0, 0.0
		if d := hi - lo; d > 0 {
			s = d / (1 - math.Abs(2*l-1))
			switch hi {
			case rr:
				h = math.Mod((gg-bb)/d, 6)
			case gg:
				h = (bb-rr)/d + 2
			default:
				h = (rr-gg)/d + 4
			}
			h *= 60
			if h < 0 {
				h += 360
			}
		}
		return fmt.Sprintf("HSL(%3.0f,%3.0f%%,%3.0f%%)", h, s*100, l*100)
	default:
		return fmt.Sprintf("RGB(%3d,%3d,%3d)", r, g, b)
	}
}
