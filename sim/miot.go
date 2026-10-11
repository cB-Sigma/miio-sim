package sim

import (
	"fmt"
)

// miot answers MIoT requests the way devices do: get_properties gives
// each asked property with code 0 and its value, or code -4003 for one
// the device does not have; set_properties answers a code per property
// (-4003 unknown, -4002 not writable, -4005 a value of the wrong kind);
// action answers code 0 or -4003. Every answer repeats the request's did.
// The caller holds d.mu.
func (d *Device) miot(method string, p []any) any {
	key := func(m map[string]any, id string) string {
		return fmt.Sprintf("%v.%v", m["siid"], m[id])
	}
	switch method {
	case "get_properties", "set_properties":
		out := make([]any, 0, len(p))
		for _, x := range p {
			m, _ := x.(map[string]any)
			k := key(m, "piid")
			r := map[string]any{"did": m["did"], "siid": m["siid"], "piid": m["piid"]}
			cur, has := d.MIoT[k]
			switch {
			case !has:
				r["code"] = -4003
			case method == "get_properties":
				r["code"], r["value"] = 0, cur
			case d.ReadOnly[k]:
				r["code"] = -4002
			default:
				v := m["value"]
				if fmt.Sprintf("%T", v) != fmt.Sprintf("%T", cur) {
					r["code"] = -4005
				} else {
					d.MIoT[k] = v
					r["code"] = 0
				}
			}
			out = append(out, r)
		}
		return out
	case "action":
		if len(p) != 1 {
			return devError{-5001, "invalid_arg"}
		}
		m, _ := p[0].(map[string]any)
		r := map[string]any{"did": m["did"], "siid": m["siid"], "aiid": m["aiid"], "code": -4003}
		if f, ok := d.Actions[key(m, "aiid")]; ok {
			f(d)
			r["code"] = 0
		}
		return r
	}
	return nil
}

// MIoTValue returns a MIoT property ("siid.piid").
func (d *Device) MIoTValue(k string) any {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.MIoT[k]
}
