// Package sim simulates Xiaomi devices and services for tests and demos:
// miIO devices on loopback UDP (the hello handshake, checksummed encrypted
// packets, miIO.info, get_prop and the set_* methods of plugs, lights and
// air purifiers, and the MIoT get_properties, set_properties and action
// requests), the Mi account (Cloud: sign-in and the device list with
// tokens), and a MIoT specification service (SpecServer). Built from the
// protocol facts in the parent package; not a Xiaomi product.
package sim

import (
	"encoding/hex"
	"encoding/json"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/cB-Sigma/miio"
)

// Device is a simulated device.
type Device struct {
	ID    uint32
	Token []byte
	Model string
	Props map[string]any

	// DropSeenIDs ignores a request whose ID the device already answered
	// (devices do, so a client restarting its IDs at 1 is not answered).
	DropSeenIDs bool
	// MIoTOnly answers "method not found" to the legacy get_prop and set_*
	// methods, as MIoT-only models (zhimi.airpurifier.mb3 …) do.
	MIoTOnly bool
	// MIoT are the device's MIoT properties by "siid.piid" (nil: the
	// device does not speak MIoT, it answers "method not found").
	MIoT map[string]any
	// ReadOnly are MIoT properties that refuse writes (code -4002).
	ReadOnly map[string]bool
	// Actions run by "siid.aiid".
	Actions map[string]func(d *Device)

	mu    sync.Mutex
	seen  map[int]bool
	conn  *net.UDPConn
	start time.Time
	calls []string
}

// Start serves a device on 127.0.0.1, on a free port.
func Start(id uint32, tokenHex, model string, props map[string]any) (*Device, error) {
	return StartAt("127.0.0.1:0", id, tokenHex, model, props)
}

// StartAt serves a device on a UDP address (host:port), such as
// 127.0.0.1:54321 for clients that send their hello only to the miIO
// port.
func StartAt(addr string, id uint32, tokenHex, model string, props map[string]any) (*Device, error) {
	tok, err := hex.DecodeString(tokenHex)
	if err != nil {
		return nil, err
	}
	ua, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return nil, err
	}
	c, err := net.ListenUDP("udp4", ua)
	if err != nil {
		return nil, err
	}
	d := &Device{ID: id, Token: tok, Model: model, Props: props, conn: c, start: time.Now().Add(-1000 * time.Second)}
	go d.serve()
	return d, nil
}

// Addr is the device's address.
func (d *Device) Addr() string { return d.conn.LocalAddr().String() }

// Close stops the device.
func (d *Device) Close() { d.conn.Close() }

// Prop returns a property.
func (d *Device) Prop(k string) any {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.Props[k]
}

// Configure changes the device's behaviour flags under its lock.
func (d *Device) Configure(f func(d *Device)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	f(d)
}

// Calls lists the methods called.
func (d *Device) Calls() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.calls...)
}

func (d *Device) stamp() uint32 { return uint32(time.Since(d.start) / time.Second) }

func (d *Device) serve() {
	buf := make([]byte, 64<<10)
	for {
		n, from, err := d.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		pkt := buf[:n]
		if n == 32 {
			if h, err := miio.ParseHeader(pkt); err == nil && h.Unknown == 0xffffffff {
				hello := miio.Hello()
				putU32(hello[4:], 0)
				putU32(hello[8:], d.ID)
				putU32(hello[12:], d.stamp())
				_, _ = d.conn.WriteToUDP(hello, from)
				continue
			}
		}
		_, plain, err := miio.Open(d.Token, pkt)
		if err != nil {
			continue // a wrong token: real devices stay silent
		}
		var req struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
			Params any    `json:"params"`
		}
		if json.Unmarshal(plain, &req) != nil {
			continue
		}
		// An action's params are one object; every other method's a list.
		params, _ := req.Params.([]any)
		if obj, ok := req.Params.(map[string]any); ok {
			params = []any{obj}
		}
		d.mu.Lock()
		drop := d.DropSeenIDs && d.seen[req.ID]
		if d.seen == nil {
			d.seen = map[int]bool{}
		}
		d.seen[req.ID] = true
		d.mu.Unlock()
		if drop {
			continue
		}
		res := d.handle(req.Method, params)
		out, _ := json.Marshal(map[string]any{"id": req.ID, "result": res})
		if e, ok := res.(devError); ok {
			out, _ = json.Marshal(map[string]any{"id": req.ID, "error": map[string]any{"code": e.code, "message": e.msg}})
		} else if res == nil {
			out, _ = json.Marshal(map[string]any{"id": req.ID, "error": map[string]any{"code": -32601, "message": "Method not found."}})
		}
		_, _ = d.conn.WriteToUDP(miio.Packet(d.Token, d.ID, d.stamp(), out), from)
	}
}

func putU32(b []byte, v uint32) {
	b[0], b[1], b[2], b[3] = byte(v>>24), byte(v>>16), byte(v>>8), byte(v)
}

// devError is an error answer.
type devError struct {
	code int
	msg  string
}

var errOff = devError{-5000, "general error"} // what a Yeelight answers to a change while off

// handle answers a method the way the model does: a Yeelight takes
// brightness, colour temperature and colour only while on; a Philips
// light takes one argument per method and set_cct; a chuangmi.plug.v1
// switches with set_on/set_off and, like v3, reports its state as "on".
func (d *Device) handle(method string, p []any) any {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, method)
	arg := func(i int) any {
		if i < len(p) {
			return p[i]
		}
		return nil
	}
	ok := []any{"ok"}
	switch method {
	case "get_properties", "set_properties", "action":
		if d.MIoT == nil {
			return nil
		}
		return d.miot(method, p)
	}
	if d.MIoTOnly && method != "miIO.info" {
		return nil
	}
	m := d.Model
	yeelight := strings.HasPrefix(m, "yeelink.light.")
	philips := strings.HasPrefix(m, "philips.light.")
	onKey := m == "chuangmi.plug.v1" || m == "chuangmi.plug.v3"
	if philips {
		switch method {
		case "set_power", "set_bright", "set_cct":
			if len(p) != 1 {
				return devError{-5001, "invalid_arg"}
			}
		case "set_ct_abx", "set_rgb":
			return nil
		}
		if method == "set_cct" {
			d.Props["cct"] = arg(0)
			return ok
		}
	}
	if yeelight && d.Props["power"] != "on" {
		switch method {
		case "set_bright", "set_ct_abx", "set_rgb", "set_hsv":
			return errOff
		}
	}
	if onKey {
		switch method {
		case "set_power":
			if m == "chuangmi.plug.v1" {
				return nil
			}
			d.Props["on"] = arg(0) == "on"
			return ok
		case "set_on", "set_off":
			d.Props["on"] = method == "set_on"
			return ok
		}
	}
	switch method {
	case "miIO.info":
		return map[string]any{"model": d.Model, "fw_ver": "2.1.9_0023", "hw_ver": "esp32", "mac": "78:11:DC:00:00:01", "token": "hidden"}
	case "get_prop":
		out := make([]any, len(p))
		for i, k := range p {
			ks, _ := k.(string)
			out[i] = d.Props[ks]
		}
		return out
	case "set_power":
		d.Props["power"] = arg(0)
		return ok
	case "set_bright":
		d.Props["bright"] = arg(0)
		d.Props["power"] = "on"
		return ok
	case "set_ct_abx":
		d.Props["ct"], d.Props["color_mode"], d.Props["power"] = arg(0), 2.0, "on"
		return ok
	case "set_rgb":
		d.Props["rgb"], d.Props["color_mode"], d.Props["power"] = arg(0), 1.0, "on"
		return ok
	case "set_mode":
		d.Props["mode"] = arg(0)
		return ok
	case "set_level_favorite":
		d.Props["favorite_level"] = arg(0)
		return ok
	}
	return nil
}

// Demo tokens and devices.
const (
	PlugToken     = "00112233445566778899aabbccddeeff"
	LightToken    = "ffeeddccbbaa99887766554433221100"
	PurifierToken = "0f1e2d3c4b5a69788796a5b4c3d2e1f0"
)

// Demo is a plug, a Yeelight bulb and an air purifier.
type Demo struct{ Plug, Light, Purifier *Device }

// Close stops the devices.
func (d *Demo) Close() {
	for _, x := range []*Device{d.Plug, d.Light, d.Purifier} {
		if x != nil {
			x.Close()
		}
	}
}

// NewDemo starts the demo devices.
func NewDemo() (*Demo, error) {
	d := &Demo{}
	var err error
	if d.Plug, err = Start(0x0a1b2c01, PlugToken, "chuangmi.plug.m3", map[string]any{"power": "on", "temperature": 38.0}); err != nil {
		return nil, err
	}
	if d.Light, err = Start(0x0a1b2c02, LightToken, "yeelink.light.color1", map[string]any{"power": "off", "bright": 80.0, "ct": 4000.0, "rgb": 16711680.0, "color_mode": 2.0}); err != nil {
		d.Close()
		return nil, err
	}
	// The Mi Air Purifier 3H speaks only MIoT: power 2.2, fan level 2.4
	// (1-3), mode 2.5 (0 auto, 1 sleep, 2 favourite, 3 none), PM2.5 3.6,
	// humidity 3.7, temperature 3.8 (its published specification).
	if d.Purifier, err = Start(0x0a1b2c03, PurifierToken, "zhimi.airpurifier.ma4", map[string]any{}); err != nil {
		d.Close()
		return nil, err
	}
	// Under the device's lock: it is already serving.
	d.Purifier.Configure(func(p *Device) {
		p.MIoTOnly = true
		p.MIoT = map[string]any{"2.1": 0.0, "2.2": true, "2.4": 2.0, "2.5": 0.0, "3.6": 42.0, "3.7": 55.0, "3.8": 26.8, "4.3": 87.0}
		p.ReadOnly = map[string]bool{"2.1": true, "3.6": true, "3.7": true, "3.8": true, "4.3": true}
	})
	return d, nil
}
