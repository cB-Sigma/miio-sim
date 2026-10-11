// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package miio_test

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cB-Sigma/miio"
	"github.com/cB-Sigma/miio-sim/sim"
)

func urn(kind, name string) string {
	return "urn:miot-spec-v2:" + kind + ":" + name + ":00000001:sim-model:1"
}

// purifierSpec is a made-up specification matching the demo purifier's
// MIoT properties.
func purifierSpec() *miio.Spec {
	rw := []string{"read", "write", "notify"}
	ro := []string{"read", "notify"}
	return &miio.Spec{Type: urn("device", "air-purifier"), Services: []miio.Service{
		{IID: 2, Type: urn("service", "air-purifier"), Properties: []miio.Property{
			{IID: 1, Type: urn("property", "fault"), Format: "uint8", Access: ro},
			{IID: 2, Type: urn("property", "on"), Format: "bool", Access: rw},
			{IID: 4, Type: urn("property", "fan-level"), Format: "uint8", Access: rw,
				ValueList: []miio.ValueItem{{Value: 1, Description: "Level1"}, {Value: 2, Description: "Level2"}, {Value: 3, Description: "Level3"}}},
			{IID: 5, Type: urn("property", "mode"), Format: "uint8", Access: rw,
				ValueList: []miio.ValueItem{{Value: 0, Description: "Auto"}, {Value: 1, Description: "Sleep"}, {Value: 2, Description: "Favorite"}}},
		}, Actions: []miio.Action{{IID: 1, Type: urn("action", "toggle")}}},
		{IID: 3, Type: urn("service", "environment"), Properties: []miio.Property{
			{IID: 6, Type: urn("property", "pm2.5-density"), Format: "float", Access: ro, ValueRange: []float64{0, 600, 1}},
			{IID: 7, Type: urn("property", "relative-humidity"), Format: "uint8", Access: ro, ValueRange: []float64{0, 100, 1}},
			{IID: 8, Type: urn("property", "temperature"), Format: "float", Access: ro, ValueRange: []float64{-40, 125, 0.1}},
		}},
	}}
}

func client(t *testing.T, d *sim.Device, token string) *miio.Client {
	t.Helper()
	tok, _ := hex.DecodeString(token)
	return &miio.Client{Addr: d.Addr(), Token: tok, Timeout: 2 * time.Second}
}

// A specification turned into names reads and writes the purifier by
// name, refuses a value outside the value list before sending, and the
// purifier helper works through it.
// Covers: SIG-IMP-185
func TestModelAgainstDevice(t *testing.T) {
	demo, err := sim.NewDemo()
	if err != nil {
		t.Fatal(err)
	}
	defer demo.Close()
	ss := sim.StartSpecServer(map[string]miio.Spec{"zhimi.airpurifier.ma4": *purifierSpec()})
	defer ss.Close()
	m, err := miio.ModelFor(ctx, &miio.HTTPSpecs{Base: ss.URL(), HTTP: http.DefaultClient}, "zhimi.airpurifier.ma4")
	if err != nil {
		t.Fatal(err)
	}
	c := client(t, demo.Purifier, sim.PurifierToken)
	got, err := m.Get(ctx, c, "x", "air-purifier.on", "pm2.5-density", "mode")
	if err != nil || got["air-purifier.on"] != true || got["pm2.5-density"] != 42.0 || got["mode"] != 0.0 {
		t.Fatalf("get %v %v", got, err)
	}
	if err := m.Set(ctx, c, "x", "mode", "Sleep"); err != nil || demo.Purifier.MIoTValue("2.5") != 1.0 {
		t.Errorf("set mode %v, %v", err, demo.Purifier.MIoTValue("2.5"))
	}
	before := len(demo.Purifier.Calls())
	if err := m.Set(ctx, c, "x", "fan-level", 7); !errors.Is(err, miio.ErrValue) || len(demo.Purifier.Calls()) != before {
		t.Errorf("an out-of-list fan level: %v, calls %d→%d", err, before, len(demo.Purifier.Calls()))
	}
	if err := m.Set(ctx, c, "x", "pm2.5-density", 1); !errors.Is(err, miio.ErrAccess) {
		t.Errorf("a read-only write: %v", err)
	}
	p := &miio.Purifier{Family: miio.Family{Client: c, DID: "x", Model: m}}
	if err := p.SetFanLevel(ctx, 3); err != nil || demo.Purifier.MIoTValue("2.4") != 3.0 {
		t.Errorf("fan level %v %v", err, demo.Purifier.MIoTValue("2.4"))
	}
	if err := p.Off(ctx); err != nil {
		t.Error(err)
	}
	st, err := p.Status(ctx)
	if err != nil || st.On || st.Mode != "Sleep" || st.FanLevel != 3 || st.PM25 != 42 || st.Humidity != 55 || st.Temperature != 26.8 {
		t.Errorf("status %+v %v", st, err)
	}
}

// A MIoT vacuum: start, pause, stop and home run the standard actions; the
// status names the state from the specification.
// Covers: SIG-IMP-185
func TestVacuumMIoT(t *testing.T) {
	d, err := sim.Start(0x0a1b2c10, sim.PlugToken, "sim.vacuum.v1", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	set := func(state float64) func(*sim.Device) {
		return func(x *sim.Device) { x.MIoT["2.1"] = state }
	}
	d.Configure(func(x *sim.Device) {
		x.MIoTOnly = true
		x.MIoT = map[string]any{"2.1": 1.0, "3.1": 80.0}
		x.ReadOnly = map[string]bool{"2.1": true, "3.1": true}
		x.Actions = map[string]func(*sim.Device){"2.1": set(2), "2.2": set(1), "2.3": set(3), "3.1": set(4)}
	})
	sp := &miio.Spec{Type: urn("device", "vacuum"), Services: []miio.Service{
		{IID: 2, Type: urn("service", "vacuum"), Properties: []miio.Property{
			{IID: 1, Type: urn("property", "status"), Format: "uint8", Access: []string{"read"},
				ValueList: []miio.ValueItem{{Value: 1, Description: "Idle"}, {Value: 2, Description: "Sweeping"}, {Value: 3, Description: "Paused"}, {Value: 4, Description: "Go Charging"}}},
		}, Actions: []miio.Action{{IID: 1, Type: urn("action", "start-sweep")}, {IID: 2, Type: urn("action", "stop-sweeping")}, {IID: 3, Type: urn("action", "pause-sweeping")}}},
		{IID: 3, Type: urn("service", "battery"), Properties: []miio.Property{
			{IID: 1, Type: urn("property", "battery-level"), Format: "uint8", Access: []string{"read"}, ValueRange: []float64{0, 100, 1}},
		}, Actions: []miio.Action{{IID: 1, Type: urn("action", "start-charge")}}},
	}}
	m, _ := miio.NewModel(sp)
	v := &miio.Vacuum{Family: miio.Family{Client: client(t, d, sim.PlugToken), DID: "x", Model: m}}
	for _, step := range []struct {
		run  func(context.Context) error
		want string
	}{{v.Start, "Sweeping"}, {v.Pause, "Paused"}, {v.Stop, "Idle"}, {v.Home, "Go Charging"}} {
		if err := step.run(ctx); err != nil {
			t.Fatal(err)
		}
		st, err := v.Status(ctx)
		if err != nil || st.State != step.want || st.Battery != 80 {
			t.Errorf("status %+v %v, want %s", st, err, step.want)
		}
	}
}

// Legacy per-model methods: a Roborock vacuum, a chuangmi plug (v1 and
// m3), a Yeelight and a Philips light, a zhimi purifier.
// Covers: SIG-IPE-4471
func TestLegacyFamilies(t *testing.T) {
	start := func(model, token string, props map[string]any) (*sim.Device, *miio.Client) {
		d, err := sim.Start(0x0a1b2c20, token, model, props)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(d.Close)
		return d, client(t, d, token)
	}
	vd, vc := start("roborock.vacuum.s5", sim.PlugToken, map[string]any{"state": 8.0, "battery": 100.0})
	v := &miio.Vacuum{Family: miio.Family{Client: vc}}
	if err := v.Start(ctx); err != nil || vd.Prop("state") != 5.0 {
		t.Errorf("app_start %v %v", err, vd.Prop("state"))
	}
	if st, err := v.Status(ctx); err != nil || st.Code != 5 || st.State != "cleaning" || st.Battery != 100 {
		t.Errorf("status %+v %v", st, err)
	}
	if err := v.Home(ctx); err != nil || vd.Prop("state") != 6.0 {
		t.Errorf("app_charge %v", err)
	}

	pd, pc := start("chuangmi.plug.v1", sim.PlugToken, map[string]any{"on": false, "temperature": 30.0})
	plug := &miio.Plug{Family: miio.Family{Client: pc, LegacyModel: "chuangmi.plug.v1"}}
	if err := plug.On(ctx); err != nil || pd.Prop("on") != true {
		t.Errorf("v1 on %v", err)
	}
	if st, err := plug.Status(ctx); err != nil || !st.On || st.Temperature != 30 {
		t.Errorf("v1 status %+v %v", st, err)
	}
	md, mc := start("chuangmi.plug.m3", sim.PlugToken, map[string]any{"power": "on"})
	m3 := &miio.Plug{Family: miio.Family{Client: mc, LegacyModel: "chuangmi.plug.m3"}}
	if err := m3.Off(ctx); err != nil || md.Prop("power") != "off" {
		t.Errorf("m3 off %v", err)
	}

	yd, yc := start("yeelink.light.color1", sim.LightToken, map[string]any{"power": "off", "bright": 10.0, "ct": 2700.0, "rgb": 0.0})
	y := &miio.Light{Family: miio.Family{Client: yc, LegacyModel: "yeelink.light.color1"}}
	if err := y.On(ctx); err != nil {
		t.Fatal(err)
	}
	if err := y.SetColorTemperature(ctx, 4000); err != nil || yd.Prop("ct") != 4000.0 {
		t.Errorf("ct %v %v", err, yd.Prop("ct"))
	}
	if err := y.SetColor(ctx, 0x00ff00); err != nil {
		t.Error(err)
	}
	if st, err := y.Status(ctx); err != nil || !st.On || st.ColorTemperature != 4000 || st.Color != 0x00ff00 {
		t.Errorf("yeelight %+v %v", st, err)
	}
	hd, hc := start("philips.light.bulb", sim.LightToken, map[string]any{"power": "on", "bright": 50.0, "cct": 10.0})
	ph := &miio.Light{Family: miio.Family{Client: hc, LegacyModel: "philips.light.bulb"}}
	if err := ph.SetBrightness(ctx, 70); err != nil || hd.Prop("bright") != 70.0 {
		t.Errorf("philips bright %v", err)
	}
	if err := ph.SetColorTemperatureScale(ctx, 60); err != nil || hd.Prop("cct") != 60.0 {
		t.Errorf("philips cct %v", err)
	}
	if err := ph.SetColor(ctx, 1); !errors.Is(err, miio.ErrUnsupported) {
		t.Errorf("philips colour: %v", err)
	}

	zd, zc := start("zhimi.airpurifier.m1", sim.PurifierToken, map[string]any{"power": "on", "mode": "auto", "aqi": 12.0, "humidity": 40.0, "temp_dec": 251.0})
	z := &miio.Purifier{Family: miio.Family{Client: zc, LegacyModel: "zhimi.airpurifier.m1"}}
	if err := z.SetMode(ctx, "Silent"); err != nil || zd.Prop("mode") != "silent" {
		t.Errorf("mode %v %v", err, zd.Prop("mode"))
	}
	if st, err := z.Status(ctx); err != nil || !st.On || st.Mode != "silent" || st.PM25 != 12 || st.Temperature != 25.1 {
		t.Errorf("purifier %+v %v", st, err)
	}
	if err := z.SetFanLevel(ctx, 2); !errors.Is(err, miio.ErrUnsupported) {
		t.Errorf("legacy fan level: %v", err)
	}
}

func gatewaySubs() []*sim.SubDevice {
	return []*sim.SubDevice{
		{SID: "lumi.158d0001aaaa01", Model: "lumi.sensor_ht.v1", TypeID: 10, Firmware: 3, Battery: 87, Props: map[string]any{"temperature": 2315.0, "humidity": 4820.0}},
		{SID: "lumi.158d0001aaaa02", Model: "lumi.sensor_magnet.v2", TypeID: 3, Firmware: 5, Battery: 55, Props: map[string]any{"status": "close"}},
	}
}

// A gateway's sub-devices: listed both ways, properties read in one
// request and one by one, written, battery, unpaired.
func TestGatewaySubDevices(t *testing.T) {
	gw, err := sim.Start(0x0a1b2c30, sim.PlugToken, "lumi.gateway.v3", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	defer gw.Close()
	gw.Configure(func(d *sim.Device) { d.SubDevices = gatewaySubs() })
	g := &miio.Gateway{Client: client(t, gw, sim.PlugToken), Model: "lumi.gateway.v3"}
	subs, err := g.SubDevices(ctx)
	if err != nil || len(subs) != 2 || subs[0].Model != "lumi.sensor_ht.v1" || subs[0].TypeID != 10 || subs[1].Firmware != 5 {
		t.Fatalf("sub-devices %+v %v", subs, err)
	}
	vals, err := g.SubProps(ctx, subs[0].SID, "temperature", "humidity")
	if err != nil || len(vals) != 2 || vals[0] != 2315.0 {
		t.Errorf("props %v %v", vals, err)
	}
	if v, err := g.SubProp(ctx, subs[1].SID, "status"); err != nil || v != "close" {
		t.Errorf("prop %v %v", v, err)
	}
	if err := g.SetSubProp(ctx, subs[1].SID, "alias", "door"); err != nil || gw.SubProp(subs[1].SID, "alias") != "door" {
		t.Errorf("set %v", err)
	}
	if b, err := g.SubBattery(ctx, subs[0].SID); err != nil || b != 87 {
		t.Errorf("battery %v %v", b, err)
	}
	if _, err := g.SubProp(ctx, "lumi.nothing", "x"); err == nil {
		t.Error("an unknown sid answered")
	}
	if err := g.RemoveSubDevice(ctx, subs[1].SID); err != nil {
		t.Error(err)
	}
	if subs, _ := g.SubDevices(ctx); len(subs) != 1 {
		t.Errorf("after unpairing %+v", subs)
	}

	mg, err := sim.Start(0x0a1b2c31, sim.PlugToken, "lumi.gateway.mgl03", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	defer mg.Close()
	mg.Configure(func(d *sim.Device) { d.SubDevices = gatewaySubs() })
	g3 := &miio.Gateway{Client: client(t, mg, sim.PlugToken), Model: "lumi.gateway.mgl03"}
	if subs, err := g3.SubDevices(ctx); err != nil || len(subs) != 2 || subs[1].Model != "lumi.sensor_magnet.v2" || subs[1].TypeID != -1 {
		t.Errorf("mgl03 %+v %v", subs, err)
	}

	devs := []miio.CloudDevice{{DID: "177"}, {DID: "lumi.1", ParentID: "177"}, {DID: "lumi.2", ParentID: "9"}}
	if got := miio.GatewaySubDevices(devs, "177"); len(got) != 1 || got[0].DID != "lumi.1" {
		t.Errorf("from the account %+v", got)
	}
}

// A firmware update: the device downloads the image, checks its MD5,
// installs and reports the new version; a wrong MD5 fails.
func TestFirmwareUpdate(t *testing.T) {
	image := []byte("firmware image for the simulator")
	sum := md5.Sum(image)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(image) }))
	defer srv.Close()
	d, err := sim.Start(0x0a1b2c40, sim.PlugToken, "chuangmi.plug.m3", map[string]any{"power": "on"})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	d.Configure(func(x *sim.Device) { x.OTAVersion, x.OTAStep = "9.9.9_0001", 20*time.Millisecond })
	c := client(t, d, sim.PlugToken)
	if st, err := miio.UpdateState(ctx, c); err != nil || st != miio.OTAIdle {
		t.Fatalf("state %q %v", st, err)
	}
	if err := miio.Update(ctx, c, srv.URL+"/fw.bin", "nothex"); !errors.Is(err, miio.ErrValue) {
		t.Errorf("a bad MD5 was sent: %v", err)
	}
	if err := miio.Update(ctx, c, srv.URL+"/fw.bin", hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
	var seen []string
	wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err = miio.WaitUpdate(wctx, c, 10*time.Millisecond, func(s string, pct int) { seen = append(seen, fmt.Sprint(s, pct)) })
	if err != nil || d.FirmwareVersion() != "9.9.9_0001" {
		t.Errorf("wait %v, version %s, seen %v", err, d.FirmwareVersion(), seen)
	}
	if p, err := miio.UpdateProgress(ctx, c); err != nil || p != 100 {
		t.Errorf("progress %d %v", p, err)
	}
	if err := miio.Update(ctx, c, srv.URL+"/fw.bin", "00000000000000000000000000000000"); err != nil {
		t.Fatal(err)
	}
	if err := miio.WaitUpdate(wctx, c, 10*time.Millisecond, nil); !errors.Is(err, miio.ErrOTAFailed) {
		t.Errorf("a wrong MD5: %v", err)
	}
}

// Pushes: the push server installs a scene on the gateway for a
// sub-device's event, the gateway pushes when it happens, the server
// answers and hands the event over; removing the scene stops it.
func TestPushes(t *testing.T) {
	gw, err := sim.Start(0x0a1b2c50, sim.PlugToken, "lumi.gateway.v3", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	defer gw.Close()
	gw.Configure(func(d *sim.Device) { d.SubDevices = gatewaySubs() })
	s := &miio.PushServer{Addr: "127.0.0.1:0"}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	gw.Configure(func(d *sim.Device) { d.PushTo = s.LocalAddr().String() })
	c := client(t, gw, sim.PlugToken)
	got := make(chan miio.Push, 4)
	if err := s.Register(c, func(p miio.Push) { got <- p }); err != nil {
		t.Fatal(err)
	}
	const door = "lumi.158d0001aaaa02"
	id, err := s.Subscribe(ctx, c, miio.EventInfo{Action: "open", Extra: "[1,6,1,0,[0,1],2,0]", SourceDID: door, SourceModel: "lumi.sensor_magnet.v2"})
	if err != nil || len(gw.Scenes()) != 1 || gw.Scenes()[0] != id {
		t.Fatalf("subscribe %q %v scenes %v", id, err, gw.Scenes())
	}
	// The gateway itself as the source: its model and ID come from miIO.info.
	if _, err := s.Subscribe(ctx, c, miio.EventInfo{Action: "alarm", Extra: "[1,19,1,111,[0,1],2,0]"}); err != nil {
		t.Fatal(err)
	}
	if n, err := gw.Emit(door, "open", []any{"open"}); err != nil || n != 1 {
		t.Fatalf("emit %d %v", n, err)
	}
	select {
	case p := <-got:
		if p.Action != "open" || p.Source != door || string(p.Params) != `["open"]` {
			t.Errorf("push %+v", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no push")
	}
	if n, err := gw.Emit(strconv.FormatUint(uint64(gw.ID), 10), "alarm", nil); err != nil || n != 1 {
		t.Errorf("gateway event %d %v", n, err)
	}
	if p := <-got; p.Action != "alarm" {
		t.Errorf("push %+v", p)
	}
	if err := s.Unsubscribe(ctx, c, id); err != nil || len(s.Subscriptions(c)) != 1 {
		t.Errorf("unsubscribe %v %v", err, s.Subscriptions(c))
	}
	if _, err := gw.Emit(door, "open", nil); err == nil {
		t.Error("an event pushed after its scene was removed")
	}
}

// Devices controlled through the account: MIoT properties read and
// written, an action, a legacy method relayed; an offline device's
// property has a non-zero code.
func TestCloudControl(t *testing.T) {
	demo, err := sim.NewDemo()
	if err != nil {
		t.Fatal(err)
	}
	defer demo.Close()
	var ran atomic.Bool
	demo.Purifier.Configure(func(d *sim.Device) {
		d.Actions = map[string]func(*sim.Device){"2.1": func(*sim.Device) { ran.Store(true) }}
	})
	sc := sim.StartCloud("user@example.invalid", "pw", map[string][]miio.CloudDevice{"i2": {{DID: "300", Model: "zhimi.airpurifier.ma4"}}})
	defer sc.Close()
	sc.Link("300", demo.Purifier)
	sc.Link("100", demo.Plug)
	c := &miio.Cloud{Username: "user@example.invalid", Password: "pw", AccountBase: sc.HTTP.URL, APIBase: sc.APIBase, HTTP: sc.HTTP.Client()}
	res, err := c.CloudGetProperties(ctx, "i2", []miio.PropRef{{DID: "300", SIID: 3, PIID: 6}, {DID: "999", SIID: 2, PIID: 2}})
	if err != nil || len(res) != 2 || res[0].Value != 42.0 || res[0].Code != 0 || res[1].Code == 0 {
		t.Fatalf("get %+v %v", res, err)
	}
	if _, err := c.CloudSetProperties(ctx, "i2", []miio.PropRef{{DID: "300", SIID: 2, PIID: 2, Value: false}}); err != nil || demo.Purifier.MIoTValue("2.2") != false {
		t.Errorf("set %v", err)
	}
	var pe *miio.PropError
	if _, err := c.CloudSetProperties(ctx, "i2", []miio.PropRef{{DID: "300", SIID: 3, PIID: 6, Value: 1.0}}); !errors.As(err, &pe) || pe.Code != -4002 {
		t.Errorf("read-only set %v", err)
	}
	if err := c.CloudDoAction(ctx, "i2", "300", 2, 1, nil); err != nil || !ran.Load() {
		t.Errorf("action %v %v", err, ran.Load())
	}
	if raw, err := c.CloudCall(ctx, "i2", "100", "get_prop", []any{"power"}); err != nil || string(raw) != `["on"]` {
		t.Errorf("relayed get_prop %s %v", raw, err)
	}
	if _, err := c.CloudCall(ctx, "i2", "100", "no_such_method", nil); err == nil {
		t.Error("an unknown relayed method succeeded")
	}
}

// The account's identity check completed by e-mail and by SMS, a wrong
// code refused, a captcha read; the device list follows.
// Covers: SIG-IMP-186
func TestCloudVerification(t *testing.T) {
	sc := sim.StartCloud("user@example.invalid", "pw", map[string][]miio.CloudDevice{
		"i2": {{DID: "123456789", Token: sim.PlugToken, Model: "chuangmi.plug.m3"}, {DID: "lumi.158d0001aaaa01", Model: "lumi.sensor_ht.v1", ParentID: "123456789"}}})
	defer sc.Close()
	sc.Verify = true
	newCloud := func() *miio.Cloud {
		return &miio.Cloud{Username: "user@example.invalid", Password: "pw", AccountBase: sc.HTTP.URL, APIBase: sc.APIBase, HTTP: sc.HTTP.Client()}
	}
	for _, by := range []string{miio.ByEmail, miio.ByPhone} {
		c := newCloud()
		err := c.Login(ctx)
		var ve *miio.VerifyError
		if !errors.As(err, &ve) || ve.NotificationURL == "" || !errors.Is(err, miio.ErrCloudVerify) {
			t.Fatalf("%s: login %v", by, err)
		}
		if err := c.VerifyCode(ctx, by, "246810"); err == nil {
			t.Errorf("%s: a code was taken before one was sent", by)
		}
		if err := c.SendCode(ctx, by); err != nil || len(sc.CodesSent()) != 1 || sc.CodesSent()[0] != by {
			t.Fatalf("%s: send %v %v", by, err, sc.CodesSent())
		}
		if err := c.VerifyCode(ctx, by, "000000"); !errors.Is(err, miio.ErrCloudAuth) {
			t.Errorf("%s: a wrong code: %v", by, err)
		}
		if err := c.VerifyCode(ctx, by, "246810"); err != nil {
			t.Fatalf("%s: verify %v", by, err)
		}
		devs, err := c.Devices(ctx, []string{"i2"})
		if err != nil || len(devs) != 2 || devs[0].Token != sim.PlugToken || len(miio.GatewaySubDevices(devs, "123456789")) != 1 {
			t.Errorf("%s: devices %+v %v", by, devs, err)
		}
	}
	if sc.Verified() != 2 {
		t.Errorf("verified %d", sc.Verified())
	}

	sc.Verify, sc.Captcha = false, true
	c := newCloud()
	var ve *miio.VerifyError
	if err := c.Login(ctx); !errors.As(err, &ve) || ve.CaptchaURL == "" {
		t.Fatalf("captcha: %v", err)
	}
	img, err := c.Captcha(ctx)
	if err != nil || len(img) < 8 || string(img[1:4]) != "PNG" {
		t.Fatalf("captcha image %v", err)
	}
	c.CaptchaCode = "wrong"
	if err := c.Login(ctx); !errors.As(err, &ve) || !ve.WrongCaptcha {
		t.Errorf("a wrong captcha: %v", err)
	}
	if _, err := c.Captcha(ctx); err != nil {
		t.Fatal(err)
	}
	c.CaptchaCode = "DEMO7"
	if err := c.Login(ctx); err != nil {
		t.Errorf("captcha read: %v", err)
	}
}
