// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package sim_test

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cB-Sigma/miio"
	"github.com/cB-Sigma/miio-sim/sim"
)

func TestDevice(t *testing.T) {
	d, err := sim.NewDemo()
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	tok, _ := hex.DecodeString(sim.LightToken)
	c := &miio.Client{Addr: d.Light.Addr(), Token: tok, Timeout: 2 * time.Second}
	// A Yeelight refuses a change while off, as real ones do.
	if _, err := c.Call(ctx, "set_bright", []any{50}); err == nil {
		t.Error("set_bright while off was accepted")
	}
	if _, err := c.Call(ctx, "set_power", []any{"on"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Call(ctx, "set_bright", []any{50}); err != nil || d.Light.Prop("bright") != 50.0 {
		t.Errorf("set_bright %v, bright %v", err, d.Light.Prop("bright"))
	}
	if calls := strings.Join(d.Light.Calls(), ","); !strings.Contains(calls, "set_power") {
		t.Errorf("calls %s", calls)
	}
	// A device that drops repeated request IDs.
	d.Plug.Configure(func(x *sim.Device) { x.DropSeenIDs = true })
	ptok, _ := hex.DecodeString(sim.PlugToken)
	p1 := &miio.Client{Addr: d.Plug.Addr(), Token: ptok, Timeout: 2 * time.Second}
	if _, err := p1.Call(ctx, "get_prop", []any{"power"}); err != nil {
		t.Fatal(err)
	}
}

// StartAt serves on the address given; a bad address or token is refused.
func TestStartAt(t *testing.T) {
	d, err := sim.StartAt("127.0.0.1:0", 7, sim.PlugToken, "chuangmi.plug.m3", map[string]any{"power": "on"})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ptok, _ := hex.DecodeString(sim.PlugToken)
	c := &miio.Client{Addr: d.Addr(), Token: ptok, Timeout: 2 * time.Second}
	if raw, err := c.Call(context.Background(), "get_prop", []any{"power"}); err != nil || string(raw) != `["on"]` {
		t.Errorf("get_prop %s %v", raw, err)
	}
	if _, err := sim.StartAt("no port", 7, sim.PlugToken, "m", nil); err == nil {
		t.Error("a bad address accepted")
	}
	if _, err := sim.StartAt("127.0.0.1:0", 7, "zz", "m", nil); err == nil {
		t.Error("a bad token accepted")
	}
}

func Example() {
	d, _ := sim.NewDemo() // a plug, a Yeelight bulb and a MIoT-only air purifier
	defer d.Close()
	fmt.Println(d.Plug.Prop("power"), d.Purifier.MIoTValue("3.6"))
	// Output: on 42
}
