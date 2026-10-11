// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package sim

import (
	"strings"
)

// SubDevice is a Zigbee sub-device of a simulated gateway.
type SubDevice struct {
	SID      string // "lumi.158d..."
	Model    string // zigbee model, such as "lumi.sensor_ht.v1"
	TypeID   int    // the type number older gateways list
	Firmware int
	Battery  int
	Props    map[string]any
}

// gateway answers the sub-device methods of lumi gateways: the
// device_list property (older gateways, five values per device with the
// gateway itself first), get_device_list (lumi.gateway.mgl03 only),
// get_device_prop, get_device_prop_exp, set_device_prop, get_battery and
// remove_device. An unknown sid is refused. The caller holds d.mu.
func (d *Device) gateway(method string, p []any) (any, bool) {
	if !strings.HasPrefix(d.Model, "lumi.gateway.") && !strings.HasPrefix(d.Model, "lumi.acpartner.") {
		return nil, false
	}
	mgl03 := d.Model == "lumi.gateway.mgl03"
	find := func(sid any) *SubDevice {
		for _, s := range d.SubDevices {
			if s.SID == sid {
				return s
			}
		}
		return nil
	}
	unknown := devError{-5001, "invalid_arg"}
	switch method {
	case "get_device_list":
		if !mgl03 {
			return nil, true
		}
		out := []any{}
		for _, s := range d.SubDevices {
			out = append(out, map[string]any{"did": s.SID, "model": s.Model, "num": 1, "total": len(d.SubDevices)})
		}
		return out, true
	case "get_device_prop":
		if len(p) < 2 {
			return unknown, true
		}
		if p[0] == "lumi.0" {
			if p[1] != "device_list" || mgl03 {
				v, ok := d.Props[toString(p[1])]
				if !ok {
					return unknown, true
				}
				return []any{v}, true
			}
			out := []any{"lumi.0", 0, 0, 0, 0}
			for _, s := range d.SubDevices {
				out = append(out, s.SID, s.TypeID, 0, 0, s.Firmware)
			}
			return out, true
		}
		s := find(p[0])
		if s == nil {
			return unknown, true
		}
		out := []any{}
		for _, k := range p[1:] {
			out = append(out, s.Props[toString(k)])
		}
		return out, true
	case "get_device_prop_exp":
		var out []any
		for _, x := range p {
			l, _ := x.([]any)
			if len(l) == 0 {
				return unknown, true
			}
			s := find(l[0])
			if s == nil {
				return unknown, true
			}
			vals := []any{}
			for _, k := range l[1:] {
				vals = append(vals, s.Props[toString(k)])
			}
			out = append(out, vals)
		}
		return out, true
	case "set_device_prop":
		if len(p) != 1 {
			return unknown, true
		}
		m, _ := p[0].(map[string]any)
		s := find(m["sid"])
		if s == nil {
			return unknown, true
		}
		for k, v := range m {
			if k != "sid" {
				s.Props[k] = v
			}
		}
		return []any{"ok"}, true
	case "get_battery":
		if mgl03 {
			return nil, true
		}
		if len(p) != 1 || find(p[0]) == nil {
			return unknown, true
		}
		return []any{find(p[0]).Battery}, true
	case "remove_device":
		for i, s := range d.SubDevices {
			if len(p) == 1 && s.SID == p[0] {
				d.SubDevices = append(d.SubDevices[:i], d.SubDevices[i+1:]...)
				return []any{"ok"}, true
			}
		}
		return unknown, true
	}
	return nil, false
}

func toString(v any) string { s, _ := v.(string); return s }

// GatewayToken is the demo gateway's token.
const GatewayToken = "a0b1c2d3e4f5061728394a5b6c7d8e9f"

// DemoSubDevices are a weather sensor and a door sensor with made-up
// sids, for a demo gateway.
func DemoSubDevices() []*SubDevice {
	return []*SubDevice{
		{SID: "lumi.158d0000demo01", Model: "lumi.sensor_ht.v1", TypeID: 10, Firmware: 3, Battery: 87,
			Props: map[string]any{"temperature": 2315.0, "humidity": 4820.0}},
		{SID: "lumi.158d0000demo02", Model: "lumi.sensor_magnet.v2", TypeID: 3, Firmware: 5, Battery: 55,
			Props: map[string]any{}},
	}
}

// SubProp returns a sub-device's property (nil when the sub-device or
// property does not exist).
func (d *Device) SubProp(sid, name string) any {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, s := range d.SubDevices {
		if s.SID == sid {
			return s.Props[name]
		}
	}
	return nil
}

// vacuum answers the legacy Roborock methods (roborock.vacuum.* and
// rockrobo.vacuum.* models): app_start, app_stop, app_pause and
// app_charge change Props["state"] (5 cleaning, 3 idle, 10 paused, 6
// returning home; app_spot 11 spot cleaning); get_status answers one
// object with state and battery; the rest are in vacuum.go. The caller
// holds d.mu.
func (d *Device) vacuum(method string, p []any) (any, bool) {
	if !strings.HasPrefix(d.Model, "roborock.vacuum.") && !strings.HasPrefix(d.Model, "rockrobo.vacuum.") {
		return nil, false
	}
	ok := []any{"ok"}
	switch method {
	case "app_start":
		d.Props["state"] = 5.0
	case "app_stop":
		d.Props["state"] = 3.0
	case "app_pause":
		d.Props["state"] = 10.0
	case "app_charge":
		d.Props["state"] = 6.0
	case "app_spot":
		d.Props["state"] = 11.0
	case "get_status":
		st := map[string]any{"msg_ver": 2, "msg_seq": len(d.calls)}
		for k, v := range d.Props {
			st[k] = v
		}
		return []any{st}, true
	default:
		return d.vacuumMore(method, p)
	}
	return ok, true
}
