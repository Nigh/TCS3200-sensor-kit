package main

import (
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

type fakePort struct {
	rejectLow bool
	input     chan []byte
	closed    chan struct{}
	once      sync.Once
	mu        sync.Mutex
	low       uint32
	silent    bool
}

func newFake() *fakePort {
	return &fakePort{input: make(chan []byte, 32), closed: make(chan struct{}), low: 15000}
}
func (p *fakePort) Read(b []byte) (int, error) {
	select {
	case <-p.closed:
		return 0, io.EOF
	case data := <-p.input:
		return copy(b, data), nil
	case <-time.After(10 * time.Millisecond):
		return 0, nil
	}
}
func (p *fakePort) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.silent {
		return len(b), nil
	}
	var d decoder
	d.feed(b, func(f frame) {
		if f.kind == cmdStatus {
			data := make([]byte, 32)
			data[0], data[1] = 0xe8, 3
			data[2] = 250
			put32(data[4:], p.low)
			p.input <- encode(msgStatus, f.sequence, data)
		} else if f.kind == cmdLow {
			if p.rejectLow {
				p.input <- encode(msgACK, f.sequence, []byte{cmdLow, 4})
				return
			}
			p.low = uint32(f.payload[0]) | uint32(f.payload[1])<<8 | uint32(f.payload[2])<<16 | uint32(f.payload[3])<<24
			p.input <- encode(msgACK, f.sequence, []byte{cmdLow, 0})
		}
	})
	return len(b), nil
}
func put32(b []byte, v uint32) {
	for i := range 4 {
		b[i] = byte(v >> uint(8*i))
	}
}
func (p *fakePort) Close() error                       { p.once.Do(func() { close(p.closed) }); return nil }
func (p *fakePort) SetDTR(bool) error                  { return nil }
func (p *fakePort) SetReadTimeout(time.Duration) error { return nil }
func waitFor(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !fn() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(time.Millisecond)
	}
}
func stateIs(b *backend, state string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state == state
}
func TestDeviceLifecycle(t *testing.T) {
	b := newBackend()
	p := newFake()
	b.open = func(string) (port, error) { return p, nil }
	b.connect("fake")
	waitFor(t, func() bool { return stateIs(b, "已连接") })
	b.setLow(16000)
	waitFor(t, func() bool { b.mu.Lock(); defer b.mu.Unlock(); return !b.adjusting && b.status.low == 16000 })
	p.Close()
	waitFor(t, func() bool { return stateIs(b, "异常") })
	p2 := newFake()
	b.open = func(string) (port, error) { return p2, nil }
	b.connect("fake")
	waitFor(t, func() bool { return stateIs(b, "已连接") })
	b.disconnect()
	select {
	case <-p2.closed:
	default:
		t.Fatal("port not closed")
	}
	b.open = func(string) (port, error) { return nil, errors.New("permission denied") }
	b.connect("fake")
	waitFor(t, func() bool { return stateIs(b, "异常") })
}
func TestCommandTimeoutAndCancel(t *testing.T) {
	p := newFake()
	p.silent = true
	c := &connection{port: p, done: make(chan struct{}), replies: make(chan frame, 8)}
	if _, err := c.request(cmdStatus, nil); err == nil {
		t.Fatal("missing timeout")
	}
	done := make(chan error, 1)
	go func() { _, err := c.request(cmdStatus, nil); done <- err }()
	c.close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("missing cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("request leaked")
	}
}

func TestRejectedThresholdKeepsConnection(t *testing.T) {
	b := newBackend()
	p := newFake()
	p.rejectLow = true
	b.open = func(string) (port, error) { return p, nil }
	b.connect("fake")
	waitFor(t, func() bool { return stateIs(b, "已连接") })
	defer b.disconnect()
	b.setLow(16000)
	waitFor(t, func() bool { b.mu.Lock(); defer b.mu.Unlock(); return !b.adjusting })
	if !stateIs(b, "已连接") {
		t.Fatal("rejection disconnected a healthy device")
	}
}
func TestCancelledOpenCannotReplaceNewSession(t *testing.T) {
	b := newBackend()
	p := newFake()
	started, release := make(chan struct{}), make(chan struct{})
	b.open = func(string) (port, error) { close(started); <-release; return p, nil }
	b.connect("fake")
	<-started
	b.disconnect()
	close(release)
	select {
	case <-p.closed:
	case <-time.After(time.Second):
		t.Fatal("cancelled port leaked")
	}
	if !stateIs(b, "未连接") {
		t.Fatal("stale open changed state")
	}
}
