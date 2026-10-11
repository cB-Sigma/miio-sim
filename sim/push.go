// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package sim

import (
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net"
	"strings"
	"time"

	"github.com/cB-Sigma/miio"
)

// scene is a local automation installed with send_data_frame: when the
// trigger's event happens on its source, the device sends the target's
// command to the target's address.
type scene struct {
	id                  string
	source, key, extra  string
	command, ip, tokenE string
}

// scene answers send_data_frame (type "scene", data a JSON list of
// [id, ["1.0", n, ["0", trigger], [target...]]]) and miIO.xdel (a list of
// scene IDs). A malformed scene is refused. The caller holds d.mu.
func (d *Device) scene(method string, p []any) any {
	bad := devError{-5001, "invalid_arg"}
	if method == "miIO.xdel" {
		for _, x := range p {
			id := toString(x)
			for i, s := range d.scenes {
				if s.id == id {
					d.scenes = append(d.scenes[:i], d.scenes[i+1:]...)
					break
				}
			}
		}
		return []any{"ok"}
	}
	if len(p) != 1 {
		return bad
	}
	m, _ := p[0].(map[string]any)
	if toString(m["type"]) != "scene" {
		return bad
	}
	var list [][]json.RawMessage
	if json.Unmarshal([]byte(toString(m["data"])), &list) != nil || len(list) == 0 {
		return bad
	}
	for _, item := range list {
		var id string
		var body []json.RawMessage
		if len(item) != 2 || json.Unmarshal(item[0], &id) != nil || json.Unmarshal(item[1], &body) != nil || len(body) != 4 {
			return bad
		}
		var trig []json.RawMessage
		var trigger struct{ DID, Key, Extra string }
		var targets []struct{ Command, IP, Token string }
		if json.Unmarshal(body[2], &trig) != nil || len(trig) != 2 || json.Unmarshal(trig[1], &trigger) != nil ||
			json.Unmarshal(body[3], &targets) != nil || len(targets) == 0 || trigger.Key == "" {
			return bad
		}
		for _, t := range targets {
			if t.Command == "" || t.IP == "" {
				return bad
			}
			d.scenes = append(d.scenes, scene{id: id, source: trigger.DID, key: trigger.Key, extra: trigger.Extra,
				command: t.Command, ip: t.IP, tokenE: t.Token})
		}
	}
	return []any{"ok"}
}

// Scenes lists the installed scene IDs.
func (d *Device) Scenes() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []string
	for _, s := range d.scenes {
		out = append(out, s.id)
	}
	return out
}

// Emit fires an event: every scene whose trigger names the source (a
// device ID or a sub-device sid) and the event (the end of its
// "event.<model>.<event>" key) sends its command with params to its
// target, encrypted with this device's token, and waits up to a second
// for the target's answer. It returns how many targets answered.
func (d *Device) Emit(source, event string, params any) (int, error) {
	d.mu.Lock()
	var hits []scene
	for _, s := range d.scenes {
		if s.source == source && strings.HasSuffix(s.key, "."+event) {
			hits = append(hits, s)
		}
	}
	tok, id, pushTo := d.Token, d.ID, d.PushTo
	stamp := d.stamp()
	d.mu.Unlock()
	if len(hits) == 0 {
		return 0, errors.New("no scene is subscribed to that event")
	}
	answered := 0
	var firstErr error
	for _, s := range hits {
		addr := pushTo
		if addr == "" {
			addr = net.JoinHostPort(s.ip, "54321")
		}
		if err := push(addr, tok, id, stamp, s.command, params); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		answered++
	}
	return answered, firstErr
}

func push(addr string, tok []byte, id, stamp uint32, command string, params any) error {
	conn, err := net.Dial("udp4", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	reqID := 1 + rand.IntN(100000)
	req, _ := json.Marshal(map[string]any{"id": reqID, "method": command, "params": params})
	if _, err := conn.Write(miio.Packet(tok, id, stamp, req)); err != nil {
		return err
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return err
	}
	_, plain, err := miio.Open(tok, buf[:n])
	if err != nil {
		return err
	}
	var ans struct {
		ID int `json:"id"`
	}
	if json.Unmarshal(plain, &ans) != nil || ans.ID != reqID {
		return errors.New("the push target answered another request")
	}
	return nil
}
