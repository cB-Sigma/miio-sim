// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

// Package interop runs python-miio's client (peer/client.py, in a
// container) against the simulated devices: the hello handshake,
// miIO.info, legacy methods and MIoT requests. The tests need Docker and
// are skipped unless SIGMA_MIIO_INTEROP=1.
package interop

import (
	"encoding/json"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cB-Sigma/miio-sim/sim"
)

// event splits a peer line "<name> <json>" and decodes the JSON.
func event(t *testing.T, line string) any {
	t.Helper()
	_, js, _ := strings.Cut(line, " ")
	var v any
	if err := json.Unmarshal([]byte(js), &v); err != nil {
		t.Fatalf("peer line %q: %v", line, err)
	}
	return v
}

func hostPort(t *testing.T, addr string) string {
	t.Helper()
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	return port
}

// python-miio's handshake, miIO.info and legacy methods against the
// simulated plug; a wrong token goes unanswered.
func TestPythonMiioAgainstLegacyDevice(t *testing.T) {
	needDocker(t)
	d := startAtMiioPort(t, 0x0a1b2c09, sim.PlugToken, "chuangmi.plug.m3", map[string]any{"power": "on", "temperature": 38.0})
	port := hostPort(t, d.Addr())
	p := startPeer(t, "--add-host=host.docker.internal:host-gateway", image,
		"python", "-u", "client.py", "legacy", "host.docker.internal", port, sim.PlugToken)
	hello := event(t, p.expect(60*time.Second, "hello")).(map[string]any)
	if hello["device_id"] != float64(0x0a1b2c09) {
		t.Errorf("python-miio read the hello as %v", hello)
	}
	info := event(t, p.expect(15*time.Second, "info")).(map[string]any)
	if info["model"] != "chuangmi.plug.m3" || info["mac"] != "78:11:DC:00:00:01" {
		t.Errorf("info %v", info)
	}
	if got := event(t, p.expect(15*time.Second, "get_prop")); !equalJSON(got, []any{"on", 38.0}) {
		t.Errorf("get_prop %v", got)
	}
	if got := event(t, p.expect(15*time.Second, "set_power")); !equalJSON(got, []any{"ok"}) || d.Prop("power") != "off" {
		t.Errorf("set_power %v, power %v", got, d.Prop("power"))
	}
	if got := event(t, p.expect(15*time.Second, "get_prop2")); !equalJSON(got, []any{"off"}) {
		t.Errorf("get_prop after set_power %v", got)
	}
	p.expect(15*time.Second, "done")

	w := startPeer(t, "--add-host=host.docker.internal:host-gateway", image,
		"python", "-u", "client.py", "wrongtoken", "host.docker.internal", port)
	if r := event(t, w.expect(60*time.Second, "refused")).(map[string]any); r["error"] == "" {
		t.Errorf("wrong token %v", r)
	}
}

// python-miio's MIoT requests against the simulated purifier: properties
// read with codes, a write refused per property, an action.
func TestPythonMiioAgainstMiotDevice(t *testing.T) {
	needDocker(t)
	// The demo's purifier, on the miIO port.
	pur := startAtMiioPort(t, 0x0a1b2c03, sim.PurifierToken, "zhimi.airpurifier.ma4", map[string]any{})
	ran := make(chan bool, 1)
	pur.Configure(func(p *sim.Device) {
		p.MIoTOnly = true
		p.MIoT = map[string]any{"2.1": 0.0, "2.2": true, "2.4": 2.0, "2.5": 0.0, "3.6": 42.0, "3.7": 55.0, "3.8": 26.8, "4.3": 87.0}
		p.ReadOnly = map[string]bool{"2.1": true, "3.6": true, "3.7": true, "3.8": true, "4.3": true}
		p.Actions = map[string]func(*sim.Device){"4.1": func(*sim.Device) { ran <- true }}
	})
	did := strconv.FormatUint(uint64(pur.ID), 10)
	p := startPeer(t, "--add-host=host.docker.internal:host-gateway", image,
		"python", "-u", "client.py", "miot", "host.docker.internal", hostPort(t, pur.Addr()), sim.PurifierToken, did)
	got := event(t, p.expect(60*time.Second, "get_properties")).([]any)
	if len(got) != 3 {
		t.Fatalf("get_properties %v", got)
	}
	r0, r1, r2 := got[0].(map[string]any), got[1].(map[string]any), got[2].(map[string]any)
	if r0["code"] != 0.0 || r0["value"] != true || r1["value"] != 42.0 || r2["code"] != -4003.0 || r0["did"] != did {
		t.Errorf("get_properties %v", got)
	}
	set := event(t, p.expect(15*time.Second, "set_properties")).([]any)
	if len(set) != 2 || set[0].(map[string]any)["code"] != 0.0 || set[1].(map[string]any)["code"] != -4002.0 || pur.MIoTValue("2.2") != false {
		t.Errorf("set_properties %v, power %v", set, pur.MIoTValue("2.2"))
	}
	act := event(t, p.expect(15*time.Second, "action")).(map[string]any)
	if act["code"] != 0.0 {
		t.Errorf("action %v", act)
	}
	select {
	case <-ran:
	case <-time.After(5 * time.Second):
		t.Error("the action did not run")
	}
	p.expect(15*time.Second, "done")
}

// startAtMiioPort starts a simulated device on 127.0.0.1:54321: python-miio
// sends its hello only to the miIO port. The test is skipped when the port
// is taken.
func startAtMiioPort(t *testing.T, id uint32, token, model string, props map[string]any) *sim.Device {
	t.Helper()
	d, err := sim.StartAt("127.0.0.1:54321", id, token, model, props)
	if err != nil {
		t.Skipf("UDP 127.0.0.1:54321 is not free: %v", err)
	}
	t.Cleanup(d.Close)
	return d
}

func equalJSON(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
