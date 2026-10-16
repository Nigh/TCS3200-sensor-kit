package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

type device struct{ path, label string }
type port interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Close() error
	SetDTR(bool) error
	SetReadTimeout(time.Duration) error
}
type commandRejected byte

func (r commandRejected) Error() string {
	return fmt.Sprintf("设备拒绝命令，结果码 %d", byte(r))
}

type connection struct {
	port      port
	done      chan struct{}
	replies   chan frame
	once      sync.Once
	commandMu sync.Mutex
	sequence  uint16
}

func (c *connection) close() { c.once.Do(func() { close(c.done); _ = c.port.Close() }) }
func (c *connection) request(kind byte, payload []byte) (frame, error) {
	c.commandMu.Lock()
	defer c.commandMu.Unlock()
	select {
	case <-c.done:
		return frame{}, errors.New("设备已断开")
	default:
	}
	c.sequence++
	seq := c.sequence
	data := encode(kind, seq, payload)
	for len(data) > 0 {
		n, err := c.port.Write(data)
		if err != nil {
			return frame{}, err
		}
		if n == 0 {
			return frame{}, io.ErrShortWrite
		}
		data = data[n:]
	}
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	expected := byte(msgACK)
	if kind == cmdStatus {
		expected = msgStatus
	}
	if kind == cmdUSBStatus {
		expected = msgUSBStatus
	}
	for {
		select {
		case <-c.done:
			return frame{}, errors.New("设备已断开")
		case <-timer.C:
			return frame{}, errors.New("命令响应超时，请重新连接以确认设备状态")
		case f := <-c.replies:
			if f.sequence != seq {
				continue
			}
			if f.kind == msgACK {
				if len(f.payload) != 2 || f.payload[0] != kind {
					return frame{}, errors.New("ACK 格式错误")
				}
				if f.payload[1] != 0 {
					return frame{}, commandRejected(f.payload[1])
				}
			}
			if f.kind != expected {
				return frame{}, errors.New("响应类型错误")
			}
			return f, nil
		}
	}
}

type backend struct {
	sampleDirty                   bool
	lastSample                    time.Time
	resetID                       uint64
	mu                            sync.Mutex
	devices                       []device
	state, message                string
	enumerating, adjusting, dirty bool
	conn                          *connection
	generation                    uint64
	status                        deviceStatus
	usb                           usbStatus
	history                       history
	parserErrors                  uint64
	discardUntil                  uint64
	open                          func(string) (port, error)
	enumerate                     func() ([]device, error)
}

func newBackend() *backend {
	return &backend{state: "未连接", message: "点击枚举设备，选择传感器后连接", open: func(path string) (port, error) {
		return serial.Open(path, &serial.Mode{BaudRate: 115200, DataBits: 8, StopBits: serial.OneStopBit, InitialStatusBits: &serial.ModemOutputBits{DTR: true}})
	}, enumerate: func() ([]device, error) {
		ports, err := enumerator.GetDetailedPortsList()
		if err != nil {
			return nil, err
		}
		return sensorDevices(ports), nil
	}}
}
func sensorDevices(ports []*enumerator.PortDetails) []device {
	var ds []device
	for _, p := range ports {
		if p.IsUSB && strings.EqualFold(p.VID, "ACDC") && p.PID == "4015" {
			ds = append(ds, device{p.Name, fmt.Sprintf("%s · %s", p.Name, p.SerialNumber)})
		}
	}
	return ds
}

func (b *backend) scan() {
	b.mu.Lock()
	if b.enumerating {
		b.mu.Unlock()
		return
	}
	b.enumerating = true
	b.dirty = true
	b.mu.Unlock()
	go func() {
		ds, err := b.enumerate()
		b.mu.Lock()
		defer b.mu.Unlock()
		b.enumerating = false
		b.dirty = true
		if err != nil {
			b.message = "枚举失败：" + err.Error()
			return
		}
		b.devices = ds
		if len(ds) == 0 {
			b.message = "未发现 TCS3200 设备，请检查 USB 连接"
		} else {
			b.message = fmt.Sprintf("发现 %d 个设备", len(ds))
		}
	}()
}
func (b *backend) connect(path string) {
	b.mu.Lock()
	if b.conn != nil || b.state == "连接中" {
		b.mu.Unlock()
		return
	}
	b.generation++
	gen := b.generation
	b.state = "连接中"
	b.message = "正在打开 CDC 并验证协议…"
	b.dirty = true
	b.mu.Unlock()
	go func() {
		p, err := b.open(path)
		if err == nil {
			err = p.SetReadTimeout(100 * time.Millisecond)
		}
		if err == nil {
			err = p.SetDTR(true)
		}
		if err != nil {
			if p != nil {
				_ = p.Close()
			}
			b.mu.Lock()
			if gen == b.generation {
				b.state = "异常"
				b.message = "连接失败（检查设备占用及串口权限）：" + err.Error()
				b.dirty = true
			}
			b.mu.Unlock()
			return
		}
		c := &connection{port: p, done: make(chan struct{}), replies: make(chan frame, 8)}
		b.mu.Lock()
		if gen != b.generation {
			b.mu.Unlock()
			c.close()
			return
		}
		b.conn = c
		b.resetID++
		b.history = history{}
		b.parserErrors = 0
		b.discardUntil = 0
		b.status = deviceStatus{}
		b.usb = usbStatus{}
		b.mu.Unlock()
		go b.receive(c)
		status, err := b.getStatus(c)
		if err != nil {
			b.fail(c, err)
			return
		}
		b.mu.Lock()
		if b.conn == c {
			b.status = status
			b.state = "已连接"
			b.message = fmt.Sprintf("实时采集 · 最大 %d 组/秒", status.rate)
			b.dirty = true
		}
		b.mu.Unlock()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-c.done:
				return
			case <-ticker.C:
				s, err := b.getStatus(c)
				if err != nil {
					b.fail(c, err)
					return
				}
				b.mu.Lock()
				if b.conn == c {
					b.status = s
					b.dirty = true
				}
				b.mu.Unlock()
			}
		}
	}()
}
func (b *backend) getStatus(c *connection) (deviceStatus, error) {
	f, err := c.request(cmdStatus, nil)
	if err != nil {
		return deviceStatus{}, err
	}
	s, ok := decodeStatus(f.payload)
	if !ok {
		return s, errors.New("设备状态或采样率与协议 v3 不兼容")
	}

	uf, err := c.request(cmdUSBStatus, nil)
	var rejected commandRejected
	if errors.As(err, &rejected) && rejected == 5 {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	usb, ok := decodeUSBStatus(uf.payload)
	if !ok {
		return s, errors.New("USB 轮询状态格式错误")
	}
	b.mu.Lock()
	if b.conn == c {
		b.usb = usb
	}
	b.mu.Unlock()
	return s, nil
}
func (b *backend) receive(c *connection) {
	var d decoder
	buf := make([]byte, 2048)
	lastValid, lastBytes := time.Now(), time.Now()
	for {
		n, err := c.port.Read(buf)
		if n > 0 {
			lastBytes = time.Now()
			d.feed(buf[:n], func(f frame) {
				if f.kind == msgSample {
					s, ok := decodeSample(f)
					if !ok {
						d.errors++
						return
					}
					lastValid = time.Now()
					b.mu.Lock()
					if b.conn == c {
						// Unwrap even discarded samples so threshold changes preserve the time base.
						if b.history.add(s) && b.discardUntil > 0 {
							if b.history.elapsed < b.discardUntil {
								b.history.head, b.history.size = 0, 0
							} else {
								b.discardUntil = 0
							}
						}
						b.parserErrors = d.errors
						b.status.mode, b.status.state = s.mode, s.state
						b.lastSample = time.Now()
						b.sampleDirty = true
					}
					b.mu.Unlock()
				} else if f.kind == msgACK || f.kind == msgStatus || f.kind == msgUSBStatus {
					select {
					case c.replies <- f:
					default:
					}
				}
			})
		}
		if err != nil {
			b.fail(c, fmt.Errorf("USB 接收中断：%w", err))
			return
		}
		select {
		case <-c.done:
			return
		default:
		}
		if time.Since(lastBytes) >= 100*time.Millisecond {
			d.clear()
		}
		if time.Since(lastValid) > 2*time.Second {
			b.fail(c, errors.New("超过 2 秒未收到有效采样，请检查 USB 后重新连接"))
			return
		}
	}
}
func (b *backend) fail(c *connection, err error) {
	b.mu.Lock()
	if b.conn == c {
		b.conn = nil
		b.state = "异常"
		b.message = err.Error()
		b.adjusting = false
		b.dirty = true
	}
	b.mu.Unlock()
	c.close()
}
func (b *backend) disconnect() {
	b.mu.Lock()
	b.generation++
	c := b.conn
	b.conn = nil
	b.state = "未连接"
	b.message = "已断开，保留最后波形"
	b.adjusting = false
	b.dirty = true
	b.mu.Unlock()
	if c != nil {
		c.close()
	}
}
func (b *backend) setLow(value uint32) {
	value = max(lowMinHz, min(lowMaxHz, value))
	b.mu.Lock()
	c := b.conn
	if c == nil || b.state != "已连接" || b.adjusting || b.status.mode != 0 || b.status.state != 0 {
		b.message = "阈值仅可在测量模式 WAIT 状态设置为 4000–600000 Hz"
		b.dirty = true
		b.mu.Unlock()
		return
	}
	b.adjusting = true
	b.dirty = true
	b.mu.Unlock()
	go func() {
		var p [4]byte
		binary.LittleEndian.PutUint32(p[:], value)
		_, err := c.request(cmdLow, p[:])
		if err != nil {
			var rejected commandRejected
			if errors.As(err, &rejected) {
				b.mu.Lock()
				if b.conn == c {
					b.adjusting = false
					b.message = err.Error() + "；请等待设备回到测量 WAIT 状态"
					b.dirty = true
				}
				b.mu.Unlock()
				return
			}
			b.fail(c, err)
			return
		}
		status, err := b.getStatus(c)
		if err != nil {
			b.fail(c, err)
			return
		}
		if status.low != value {
			b.fail(c, errors.New("阈值回读不一致，请重新连接确认"))
			return
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.conn != c {
			return
		}
		b.status = status
		b.adjusting = false
		// STATUS uses the same boot clock. Skip until 100ms after its snapshot, including queued old captures.
		rawCutoff := uint32(status.uptime*1000) + 100000
		delta := int64(int32(rawCutoff - b.history.lastRaw))
		b.discardUntil = uint64(max(int64(b.history.elapsed)+delta, 0))
		b.resetID++
		b.history.head, b.history.size = 0, 0
		b.message = "阈值已更新；已有测试配置失效，等待新采样"
		b.dirty = true
	}()
}

// Serialize UI mutations, and always read back device state before enabling controls.
func (b *backend) mutate(run func(*connection) error) {
	b.mu.Lock()
	c := b.conn
	if c == nil || b.state != "已连接" || b.adjusting {
		b.mu.Unlock()
		return
	}
	b.adjusting, b.dirty = true, true
	b.mu.Unlock()
	go func() {
		if err := run(c); err != nil {
			var rejected commandRejected
			if !errors.As(err, &rejected) {
				b.fail(c, err)
				return
			}
			b.mu.Lock()
			if b.conn == c {
				b.message = err.Error()
				b.adjusting, b.dirty = false, true
			}
			b.mu.Unlock()
			return
		}
		status, err := b.getStatus(c)
		if err != nil {
			b.fail(c, err)
			return
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.conn == c {
			b.status = status
			b.adjusting, b.dirty = false, true
		}
	}()
}
func (b *backend) setChannels(mask byte) {
	b.mutate(func(c *connection) error { _, err := c.request(cmdChannels, []byte{mask}); return err })
}
func (b *backend) test(config []byte, mode byte) {
	b.mutate(func(c *connection) error {
		if mode == 0 {
			_, err := c.request(cmdMode, []byte{0})
			return err
		}
		if _, err := c.request(cmdConfig, config); err != nil {
			return err
		}
		_, err := c.request(cmdMode, []byte{mode})
		return err
	})
}
