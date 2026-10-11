// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package miio_test

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cB-Sigma/miio"
	"github.com/cB-Sigma/miio-sim/sim"
)

var ctx = context.Background()

// A legacy device: miIO.info and get_prop over the handshake; a wrong
// token is told apart from silence; discovery finds the device.
func TestClient(t *testing.T) {
	d, err := sim.Start(0x0a1b2c09, sim.PlugToken, "chuangmi.plug.m3", map[string]any{"power": "on", "temperature": 38.0})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	tok, _ := hex.DecodeString(sim.PlugToken)
	c := &miio.Client{Addr: d.Addr(), Token: tok, Timeout: 2 * time.Second}
	info, err := c.Call(ctx, "miIO.info", nil)
	if err != nil || !strings.Contains(string(info), "chuangmi.plug.m3") || c.DeviceID() != 0x0a1b2c09 {
		t.Fatalf("info %s %v id %x", info, err, c.DeviceID())
	}
	props, err := c.Call(ctx, "get_prop", []any{"power", "temperature"})
	if err != nil || string(props) != `["on",38]` {
		t.Errorf("get_prop %s %v", props, err)
	}
	if _, err := c.Call(ctx, "set_power", []any{"off"}); err != nil || d.Prop("power") != "off" {
		t.Errorf("set_power %v, power %v", err, d.Prop("power"))
	}
	wrong, _ := hex.DecodeString(sim.LightToken)
	w := &miio.Client{Addr: d.Addr(), Token: wrong, Timeout: 2 * time.Second}
	if _, err := w.Call(ctx, "miIO.info", nil); err == nil {
		t.Error("a wrong token was answered")
	}
	found, err := miio.Discover(ctx, d.Addr(), 300*time.Millisecond)
	if err != nil || found[0x0a1b2c09] != d.Addr() {
		t.Errorf("discovery %v %v", found, err)
	}
}

// MIoT requests: properties read in batches, a write refused per property,
// an action.
func TestMIoT(t *testing.T) {
	demo, err := sim.NewDemo()
	if err != nil {
		t.Fatal(err)
	}
	defer demo.Close()
	var ran atomic.Bool
	demo.Purifier.Configure(func(d *sim.Device) {
		d.Actions = map[string]func(*sim.Device){"4.1": func(*sim.Device) { ran.Store(true) }}
	})
	tok, _ := hex.DecodeString(sim.PurifierToken)
	c := &miio.Client{Addr: demo.Purifier.Addr(), Token: tok, Timeout: 2 * time.Second}
	var refs []miio.PropRef
	for i := 0; i < 20; i++ { // more than one request's worth
		refs = append(refs, miio.PropRef{DID: "x", SIID: 2, PIID: 2})
	}
	refs = append(refs, miio.PropRef{DID: "x", SIID: 9, PIID: 9})
	res, err := miio.GetProperties(ctx, c, refs)
	if err != nil || len(res) != 21 || res[0].Value != true || res[20].Code != -4003 {
		t.Fatalf("get_properties %d %+v %v", len(res), res, err)
	}
	if _, err := miio.SetProperties(ctx, c, []miio.PropRef{{DID: "x", SIID: 2, PIID: 2, Value: false}}); err != nil || demo.Purifier.MIoTValue("2.2") != false {
		t.Errorf("set %v, power %v", err, demo.Purifier.MIoTValue("2.2"))
	}
	_, err = miio.SetProperties(ctx, c, []miio.PropRef{{DID: "x", SIID: 3, PIID: 6, Value: 1.0}})
	var pe *miio.PropError
	if !errors.As(err, &pe) || pe.Code != -4002 || pe.SIID != 3 || pe.IID != 6 {
		t.Errorf("read-only write: %v", err)
	}
	if err := miio.DoAction(ctx, c, "x", 4, 1, nil); err != nil || !ran.Load() {
		t.Errorf("action %v ran %v", err, ran.Load())
	}
	if err := miio.DoAction(ctx, c, "x", 4, 9, nil); !errors.As(err, &pe) || !pe.Action {
		t.Errorf("unknown action: %v", err)
	}
	if _, err := c.Call(ctx, "get_prop", []any{"power"}); err == nil {
		t.Error("a MIoT-only model answered get_prop")
	}
}

// The Mi account: sign-in, the device list with tokens, a wrong password,
// a verification the server cannot give.
func TestCloud(t *testing.T) {
	sc := sim.StartCloud("user@example.invalid", "pw", map[string][]miio.CloudDevice{
		"i2": {{DID: "123456789", Token: sim.PlugToken, Name: "Plug", Model: "chuangmi.plug.m3", LocalIP: "192.168.1.50", Online: true}},
	})
	defer sc.Close()
	c := &miio.Cloud{Username: "user@example.invalid", Password: "pw", AccountBase: sc.HTTP.URL, APIBase: sc.APIBase, HTTP: sc.HTTP.Client()}
	devs, err := c.Devices(ctx, []string{"i2", "de"})
	if err != nil || len(devs) != 1 || devs[0].Token != sim.PlugToken || devs[0].Region != "i2" || sc.Logins() != 1 {
		t.Fatalf("devices %+v %v logins %d", devs, err, sc.Logins())
	}
	sc.EndSession()
	if devs, err := c.Devices(ctx, []string{"i2"}); err != nil || len(devs) != 1 {
		t.Errorf("after the session ended: %v %v", devs, err)
	}
	bad := &miio.Cloud{Username: "user@example.invalid", Password: "wrong", AccountBase: sc.HTTP.URL, APIBase: sc.APIBase, HTTP: sc.HTTP.Client()}
	if err := bad.Login(ctx); !errors.Is(err, miio.ErrCloudAuth) {
		t.Errorf("wrong password: %v", err)
	}
	sc.Verify = true
	v := &miio.Cloud{Username: "user@example.invalid", Password: "pw", AccountBase: sc.HTTP.URL, APIBase: sc.APIBase, HTTP: sc.HTTP.Client()}
	if err := v.Login(ctx); !errors.Is(err, miio.ErrCloudVerify) {
		t.Errorf("verification: %v", err)
	}
}

// Specifications come from the specification service by model, newest
// version first; an unknown model is ErrNoSpec.
func TestHTTPSpecs(t *testing.T) {
	spec := miio.Spec{Type: "urn:miot-spec-v2:device:light:0000A001:test-lamp:1", Services: []miio.Service{
		{IID: 2, Type: "urn:miot-spec-v2:service:light:00007802:test-lamp:1", Properties: []miio.Property{
			{IID: 1, Type: "urn:miot-spec-v2:property:on:00000006:test-lamp:1", Format: "bool", Access: []string{"read", "write"}}}}}}
	ss := sim.StartSpecServer(map[string]miio.Spec{"test.light.lamp1": spec})
	defer ss.Close()
	s := &miio.HTTPSpecs{Base: ss.URL(), HTTP: http.DefaultClient}
	got, err := s.Spec(ctx, "test.light.lamp1")
	if err != nil || len(got.Services) != 1 {
		t.Fatalf("spec %+v %v", got, err)
	}
	p := got.Services[0].Properties[0]
	if name, std := miio.URNName(p.Type); name != "on" || !std || !p.Can("write") || p.Can("notify") {
		t.Errorf("property %+v", p)
	}
	if _, err := s.Spec(ctx, "no.such.model"); !errors.Is(err, miio.ErrNoSpec) {
		t.Errorf("unknown model: %v", err)
	}
	if _, std := miio.URNName("urn:vendor-spec:property:x:1:y:1"); std {
		t.Error("a vendor URN taken for standard")
	}
}
