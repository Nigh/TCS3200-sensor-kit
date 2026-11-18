package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

func (s *app) spectrumView(c *ui.Context, ch int, name string, color ui.Color) {
	sp := &s.spectra[ch]
	ui.Column(c).Grow(1).Basis(0).MinWidth(0).Padding(10).Background(s.theme.Surface).Radius(8).Children(func() {
		if sp.n == 0 {
			ui.Column(c).Grow(1).Center().Label(name + "频谱").Children(func() {
				ui.Text(c, sp.message).FontSize(12).TextColor(s.theme.TextMuted)
			})
			return
		}
		xscale := chart.NewLinear(0, sp.rate/2)
		xscale.Format = func(v float64) string {
			if v >= 1000 {
				return fmt.Sprintf("%.3g kHz", v/1000)
			}
			return fmt.Sprintf("%.3g Hz", v)
		}
		yscale := chart.NewLinear(0, max(1e-6, sp.maximum*1.2))
		yscale.Format = func(v float64) string { return fmt.Sprintf("%.2g", v) }
		xticks, yticks := xscale.Ticks(4), yscale.Ticks(3)
		chart.Plot(c, chart.Axes{Left: yticks, Bottom: xticks}, func(area ui.Element) {
			area.Label(name + "频谱").Description(fmt.Sprintf("单边幅值谱，单位 μs；最近 %d 点；实际采样率 %.3g Hz；频率分辨率 %.3g Hz；去直流、Hann 窗；标注最突出峰值", sp.n, sp.rate, sp.rate/float64(sp.n))).Focusable()
			area.Draw(func(p *ui.Painter, r ui.Rect) {
				for _, tick := range yticks {
					y := chart.Y(r, tick.At)
					p.Line(r.X, y, r.X+r.W, y, 1, ui.Hex("#343434"))
				}
				for _, tick := range xticks {
					x := chart.X(r, tick.At)
					p.Line(x, r.Y, x, r.Y+r.H, 1, ui.Hex("#303030"))
				}
				sp.points = sp.points[:0]
				for i, amplitude := range sp.amplitudes {
					x := float32(i) / float32(sp.n/2)
					sp.points = append(sp.points, [2]float32{chart.X(r, x), chart.Y(r, yscale.At(amplitude))})
				}
				p.Clip(r, 0, func() { p.StrokePolyline(sp.points, 1.5, color) })
			})
			if sp.hasPeak {
				x, y := xscale.At(sp.peakHz), yscale.At(sp.peakAmplitude)
				anchor := chart.Point(c, x, y, 5, 5).Background(color).Radius(3).PassThrough()
				ui.Overlay(c, func() {
					ui.Box(c).AttachTo(anchor, ui.AnchorTop, ui.AnchorBottom).Margin(5).
						PassThrough().Role(ui.RoleTooltip).Label(name+"峰值频率").Padding(4, 8).
						Background(s.theme.Background).Border(1, s.theme.Border).Radius(6).
						Children(func() { ui.Textf(c, "峰值 %.2f Hz", sp.peakHz).FontSize(12).TextColor(color) })
				})
			}
		}).Grow(1).MinHeight(48).Gap(6)
	})
}
