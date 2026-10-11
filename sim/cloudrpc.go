// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package sim

import (
	"encoding/json"
	"strings"
)

// Link lets the account reach a simulated device under a device ID, for
// miotspec/prop/get, miotspec/prop/set, miotspec/action and home/rpc/<did>.
// A device ID not linked answers code -2 per property (or an error for an
// action or a relayed method), as for a device that is offline.
func (c *Cloud) Link(did string, d *Device) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.linked == nil {
		c.linked = map[string]*Device{}
	}
	c.linked[did] = d
}

// relay answers the device-control paths through the linked devices; nil
// for an unknown path. The caller holds c.mu.
func (c *Cloud) relay(path string, data []byte) any {
	var in struct {
		Params json.RawMessage `json:"params"`
		Method string          `json:"method"`
	}
	if json.Unmarshal(data, &in) != nil {
		return nil
	}
	switch path {
	case "/miotspec/prop/get", "/miotspec/prop/set":
		var list []map[string]any
		if json.Unmarshal(in.Params, &list) != nil {
			return nil
		}
		method := "get_properties"
		if strings.HasSuffix(path, "/set") {
			method = "set_properties"
		}
		out := []any{}
		for _, p := range list {
			d := c.linked[toString(p["did"])]
			if d == nil {
				out = append(out, map[string]any{"did": p["did"], "siid": p["siid"], "piid": p["piid"], "code": -2})
				continue
			}
			if r, ok := d.handle(method, []any{p}).([]any); ok && len(r) == 1 {
				out = append(out, r[0])
			} else {
				out = append(out, map[string]any{"did": p["did"], "siid": p["siid"], "piid": p["piid"], "code": -4003})
			}
		}
		return out
	case "/miotspec/action":
		var p map[string]any
		if json.Unmarshal(in.Params, &p) != nil {
			return nil
		}
		d := c.linked[toString(p["did"])]
		if d == nil {
			return map[string]any{"did": p["did"], "siid": p["siid"], "aiid": p["aiid"], "code": -2}
		}
		return d.handle("action", []any{p})
	}
	if did, ok := strings.CutPrefix(path, "/home/rpc/"); ok {
		d := c.linked[did]
		if d == nil || in.Method == "" {
			return nil
		}
		var params any
		_ = json.Unmarshal(in.Params, &params)
		list, _ := params.([]any)
		if obj, isObj := params.(map[string]any); isObj {
			list = []any{obj}
		}
		res := d.handle(in.Method, list)
		if _, isErr := res.(devError); isErr {
			return nil
		}
		return res
	}
	return nil
}
