// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package miio_test

import (
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/cB-Sigma/miio"
	"github.com/cB-Sigma/miio-sim/sim"
)

// The tests of what v0.3.0 added: the vacuum's legacy extras, the
// specification mirror against the simulated service, mDNS discovery
// through a browser, Wi-Fi setup, the MIoT probe and get_prop in slices.

func roborock(t *testing.T, objects bool) (*sim.Device, *miio.Vacuum) {
	t.Helper()
	d, err := sim.Start(0x0a1b2c20, sim.PlugToken, "roborock.vacuum.s5", map[string]any{"state": 8.0, "battery": 100.0})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	d.Configure(func(x *sim.Device) {
		x.Vacuum = sim.NewVacuumState()
		x.Vacuum.ObjectAnswers = objects
	})
	return d, &miio.Vacuum{Family: miio.Family{Client: client(t, d, sim.PlugToken), LegacyModel: "roborock.vacuum.s5"}}
}

func vacState(d *sim.Device) sim.VacuumState {
	var v sim.VacuumState
	d.Configure(func(x *sim.Device) { v = *x.Vacuum; v.Timers = append([]sim.VacuumTimer(nil), x.Vacuum.Timers...) })
	return v
}

// Consumables, their reset, the history in both answer shapes, a record.
func TestVacuumHistoryAndConsumables(t *testing.T) {
	for _, objects := range []bool{false, true} {
		_, v := roborock(t, objects)
		c, err := v.Consumables(ctx)
		if err != nil || c.MainBrush != 10*time.Hour || c.Filter != 5*time.Hour || c.Sensors != 2*time.Hour || c.Raw["side_brush_work_time"] != 36000.0 {
			t.Fatalf("consumables %+v %v", c, err)
		}
		if err := v.ResetConsumable(ctx, miio.Filter); err != nil {
			t.Fatal(err)
		}
		if c, _ := v.Consumables(ctx); c.Filter != 0 || c.MainBrush == 0 {
			t.Errorf("after reset %+v", c)
		}
		if err := v.ResetConsumable(ctx, miio.Consumable("no_such_part")); err == nil {
			t.Error("an unknown part was reset")
		}
		s, err := v.CleanSummary(ctx)
		if err != nil || s.Count != 2 || s.Duration != 2940*time.Second || s.Area != 53.75 || len(s.Records) != 2 || s.Records[0] != 1760000000 {
			t.Fatalf("objects %v: summary %+v %v", objects, s, err)
		}
		r, err := v.CleanRecord(ctx, s.Records[1])
		if err != nil || r.Duration != 840*time.Second || r.Area != 11.25 || r.Error != 3 || r.Complete || r.Start.Unix() != 1759900000 || r.End.Unix() != 1759900900 {
			t.Errorf("objects %v: record %+v %v", objects, r, err)
		}
		if r, err := v.CleanRecord(ctx, s.Records[0]); err != nil || !r.Complete {
			t.Errorf("objects %v: record %+v %v", objects, r, err)
		}
		if _, err := v.CleanRecord(ctx, 42); err == nil {
			t.Error("a missing record was read")
		}
	}
}

// Do-not-disturb, timers, time zone, volume, fan speed, carpet mode,
// find and spot.
func TestVacuumSettings(t *testing.T) {
	d, v := roborock(t, false)
	dnd, err := v.DND(ctx)
	if err != nil || !dnd.Enabled || dnd.StartHour != 22 || dnd.EndHour != 7 {
		t.Fatalf("dnd %+v %v", dnd, err)
	}
	if err := v.SetDND(ctx, 23, 30, 6, 15); err != nil {
		t.Fatal(err)
	}
	if dnd, _ := v.DND(ctx); dnd != (miio.DND{Enabled: true, StartHour: 23, StartMinute: 30, EndHour: 6, EndMinute: 15}) {
		t.Errorf("dnd after set %+v", dnd)
	}
	if err := v.SetDND(ctx, 24, 0, 6, 0); err == nil {
		t.Error("hour 24 was sent")
	}
	if err := v.DisableDND(ctx); err != nil || vacState(d).DNDEnabled {
		t.Errorf("disable dnd %v", err)
	}

	ts, err := v.Timers(ctx)
	if err != nil || len(ts) != 1 || ts[0].ID != "1760000000123" || !ts[0].Enabled || ts[0].Cron != "30 9 * * 1,3,5" || ts[0].Action != "start_clean" {
		t.Fatalf("timers %+v %v", ts, err)
	}
	id, err := v.AddTimer(ctx, "", "0 18 * * 6")
	if err != nil || id == "" {
		t.Fatal(id, err)
	}
	if err := v.EnableTimer(ctx, id, false); err != nil {
		t.Fatal(err)
	}
	ts, _ = v.Timers(ctx)
	if len(ts) != 2 || ts[1].ID != id || ts[1].Enabled || ts[1].Cron != "0 18 * * 6" {
		t.Errorf("timers after add %+v", ts)
	}
	if err := v.DeleteTimer(ctx, ts[0].ID); err != nil {
		t.Fatal(err)
	}
	if ts, _ := v.Timers(ctx); len(ts) != 1 || ts[0].ID != id {
		t.Errorf("timers after delete %+v", ts)
	}
	if err := v.DeleteTimer(ctx, "nope"); err == nil {
		t.Error("an unknown timer was deleted")
	}
	if _, err := v.AddTimer(ctx, "1", "not cron"); err == nil {
		t.Error("a bad schedule was accepted")
	}
	if tz, err := v.Timezone(ctx); err != nil || tz != "Asia/Kolkata" {
		t.Errorf("time zone %q %v", tz, err)
	}

	if vol, err := v.Volume(ctx); err != nil || vol != 70 {
		t.Errorf("volume %d %v", vol, err)
	}
	if err := v.SetVolume(ctx, 35); err != nil {
		t.Fatal(err)
	}
	if vol, _ := v.Volume(ctx); vol != 35 {
		t.Errorf("volume after set %d", vol)
	}
	if err := v.SetVolume(ctx, 101); err == nil {
		t.Error("volume 101 was sent")
	}
	if err := v.TestVolume(ctx); err != nil || vacState(d).VolumeTests != 1 {
		t.Errorf("test volume %v", err)
	}

	if f, err := v.FanSpeed(ctx); err != nil || f != 60 {
		t.Errorf("fan %d %v", f, err)
	}
	if err := v.SetFanSpeed(ctx, 102); err != nil {
		t.Fatal(err)
	}
	if f, _ := v.FanSpeed(ctx); f != 102 {
		t.Errorf("fan after set %d", f)
	}
	if _, err := v.FanSpeedPresets(); !errors.Is(err, miio.ErrUnsupported) {
		t.Errorf("legacy presets: %v", err)
	}

	cm, err := v.CarpetMode(ctx)
	if err != nil || !cm.Enabled || cm.StallTime != 10 || cm.CurrentHigh != 500 {
		t.Fatalf("carpet %+v %v", cm, err)
	}
	if err := v.SetCarpetMode(ctx, false); err != nil {
		t.Fatal(err)
	}
	if cm, _ := v.CarpetMode(ctx); cm.Enabled || cm.CurrentLow != 400 || cm.CurrentIntegral != 450 {
		t.Errorf("carpet after off %+v (thresholds must be kept)", cm)
	}

	if err := v.Find(ctx); err != nil || vacState(d).Found != 1 {
		t.Errorf("find %v", err)
	}
	if err := v.Spot(ctx); err != nil {
		t.Fatal(err)
	}
	if st, _ := v.Status(ctx); st.Code != 11 || st.State != "spot cleaning" {
		t.Errorf("after spot %+v", st)
	}
}

// Through a Model: fan presets from the value list, the fan level, find
// by the identify action; legacy-only functions answer ErrUnsupported.
func TestVacuumExtrasMIoT(t *testing.T) {
	d, err := sim.Start(0x0a1b2c21, sim.PlugToken, "sim.vacuum.v2", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	found := make(chan bool, 1)
	d.Configure(func(x *sim.Device) {
		x.MIoTOnly = true
		x.MIoT = map[string]any{"2.1": 1.0, "2.4": 1.0}
		x.Actions = map[string]func(*sim.Device){"4.1": func(*sim.Device) { found <- true }}
	})
	sp := &miio.Spec{Type: urn("device", "vacuum"), Services: []miio.Service{
		{IID: 2, Type: urn("service", "vacuum"), Properties: []miio.Property{
			{IID: 1, Type: urn("property", "status"), Format: "uint8", Access: []string{"read"}},
			{IID: 4, Type: urn("property", "fan-level"), Format: "uint8", Access: []string{"read", "write"},
				ValueList: []miio.ValueItem{{Value: 0, Description: "Silent"}, {Value: 1, Description: "Standard"}, {Value: 2, Description: "Strong"}}},
		}},
		{IID: 4, Type: urn("service", "identify"), Actions: []miio.Action{{IID: 1, Type: urn("action", "identify")}}},
	}}
	m, _ := miio.NewModel(sp)
	v := &miio.Vacuum{Family: miio.Family{Client: client(t, d, sim.PlugToken), DID: "x", Model: m}}
	pr, err := v.FanSpeedPresets()
	if err != nil || len(pr) != 3 || pr["Strong"] != 2 {
		t.Errorf("presets %v %v", pr, err)
	}
	if err := v.SetFanSpeed(ctx, pr["Strong"]); err != nil || d.MIoTValue("2.4") != 2.0 {
		t.Errorf("set fan %v %v", err, d.MIoTValue("2.4"))
	}
	if f, err := v.FanSpeed(ctx); err != nil || f != 2 {
		t.Errorf("fan %d %v", f, err)
	}
	if err := v.SetFanSpeed(ctx, 9); !errors.Is(err, miio.ErrValue) {
		t.Errorf("a fan level outside the list: %v", err)
	}
	if err := v.Find(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-found:
	case <-time.After(3 * time.Second):
		t.Error("identify did not run")
	}
	for name, f := range map[string]func() error{
		"consumables": func() error { _, err := v.Consumables(ctx); return err },
		"summary":     func() error { _, err := v.CleanSummary(ctx); return err },
		"dnd":         func() error { _, err := v.DND(ctx); return err },
		"timers":      func() error { _, err := v.Timers(ctx); return err },
		"volume":      func() error { _, err := v.Volume(ctx); return err },
		"carpet":      func() error { _, err := v.CarpetMode(ctx); return err },
		"spot":        func() error { return v.Spot(ctx) },
	} {
		if err := f(); !errors.Is(err, miio.ErrUnsupported) {
			t.Errorf("%s through a Model: %v", name, err)
		}
	}
}

// A mirror filled from the simulated service serves the model offline,
// to the library directly and to HTTPSpecs through its handler; the
// purifier helper then drives the simulated purifier.
func TestSpecMirrorAgainstSpecServer(t *testing.T) {
	demo, err := sim.NewDemo()
	if err != nil {
		t.Fatal(err)
	}
	defer demo.Close()
	sp := purifierSpec()
	ss := sim.StartSpecServer(map[string]miio.Spec{"zhimi.airpurifier.ma4": *sp})
	defer ss.Close()
	ss.SetTranslations(sp.Type, miio.Translations{"hi": {miio.TextKey(2, 5, -1, 1): "नींद"}})
	dir := t.TempDir()
	m := &miio.SpecMirror{Dir: dir, Upstream: &miio.HTTPSpecs{Base: ss.URL(), HTTP: http.DefaultClient, Status: "all"}}
	if err := m.Fill(ctx, "zhimi.airpurifier.ma4"); err != nil {
		t.Fatal(err)
	}
	if err := m.FillTranslations(ctx, "zhimi.airpurifier.ma4"); err != nil {
		t.Fatal(err)
	}
	ss.SetDown(true)
	hits := ss.Hits()

	srv := httptest.NewServer((&miio.SpecMirror{Dir: dir}).Handler())
	defer srv.Close()
	model, err := miio.ModelFor(ctx, &miio.HTTPSpecs{Base: srv.URL + "/miot-spec-v2", HTTP: srv.Client()}, "zhimi.airpurifier.ma4")
	if err != nil {
		t.Fatal(err)
	}
	p := &miio.Purifier{Family: miio.Family{Client: client(t, demo.Purifier, sim.PurifierToken), DID: "x", Model: model}}
	if err := p.SetMode(ctx, "Favorite"); err != nil || demo.Purifier.MIoTValue("2.5") != 2.0 {
		t.Errorf("mode through the mirrored spec %v %v", err, demo.Purifier.MIoTValue("2.5"))
	}
	direct, err := miio.ModelFor(ctx, &miio.SpecMirror{Dir: dir, Upstream: &miio.HTTPSpecs{Base: ss.URL()}}, "zhimi.airpurifier.ma4")
	if err != nil || !direct.Has("air-purifier.mode") {
		t.Errorf("the mirror with its upstream down: %v", err)
	}
	tr, err := (&miio.HTTPSpecs{Base: srv.URL + "/miot-spec-v2"}).Translations(ctx, sp.Type)
	if err != nil || tr.Text("hi", "service:002:property:005:valuelist:001") != "नींद" {
		t.Errorf("translations %v %v", tr, err)
	}
	if ss.Hits() != hits {
		t.Errorf("the mirror asked the down service %d times", ss.Hits()-hits)
	}
}

// mDNS discovery through a browser finds the devices with model and ID;
// the client then talks to them.
func TestDiscoverMDNSWithSim(t *testing.T) {
	demo, err := sim.NewDemo()
	if err != nil {
		t.Fatal(err)
	}
	defer demo.Close()
	if demo.Plug.MDNSInstance() != "chuangmi-plug-m3_miio"+strconv.FormatUint(uint64(demo.Plug.ID), 10) {
		t.Errorf("instance %s", demo.Plug.MDNSInstance())
	}
	found, err := miio.DiscoverMDNS(ctx, sim.Browser(demo.Plug, demo.Light))
	if err != nil || len(found) != 2 {
		t.Fatalf("found %+v %v", found, err)
	}
	f := found[1]
	if f.Model != "yeelink.light.color1" || f.DID != demo.Light.ID || f.Addr != demo.Light.Addr() {
		t.Errorf("light %+v", f)
	}
	tok, _ := hex.DecodeString(sim.LightToken)
	c := &miio.Client{Addr: f.Addr, Token: tok, Timeout: 2 * time.Second}
	if _, err := c.Call(ctx, "miIO.info", nil); err != nil || c.DeviceID() != f.DID {
		t.Errorf("client on the found address %v, ID %d", err, c.DeviceID())
	}
}

// Wi-Fi setup, the MIoT probe and get_prop in slices.
func TestDeviceRequests(t *testing.T) {
	demo, err := sim.NewDemo()
	if err != nil {
		t.Fatal(err)
	}
	defer demo.Close()
	pc := client(t, demo.Plug, sim.PlugToken)
	if err := miio.ConfigureWiFi(ctx, pc, "site-net", "demo-wifi-pass", 1234, map[string]any{"country_domain": "in"}); err != nil {
		t.Fatal(err)
	}
	var wifi map[string]any
	demo.Plug.Configure(func(x *sim.Device) { wifi = x.WiFi })
	if wifi["ssid"] != "site-net" || wifi["passwd"] != "demo-wifi-pass" || wifi["uid"] != 1234.0 || wifi["country_domain"] != "in" {
		t.Errorf("config_router %v", wifi)
	}
	if ok, err := miio.SupportsMIoT(ctx, pc); ok || err != nil {
		t.Errorf("legacy plug: MIoT %v %v", ok, err)
	}
	if ok, err := miio.SupportsMIoT(ctx, client(t, demo.Purifier, sim.PurifierToken)); !ok || err != nil {
		t.Errorf("purifier: MIoT %v %v", ok, err)
	}
	lc := client(t, demo.Light, sim.LightToken)
	before := len(demo.Light.Calls())
	vals, err := miio.GetProp(ctx, lc, []string{"power", "bright", "ct", "rgb", "color_mode"}, 2)
	if err != nil || len(vals) != 5 || vals[0] != "off" || vals[1] != 80.0 || vals[4] != 2.0 {
		t.Errorf("get_prop %v %v", vals, err)
	}
	if n := len(demo.Light.Calls()) - before; n != 3 {
		t.Errorf("%d get_prop requests, want 3", n)
	}
	var de *miio.DeviceError
	if _, err := lc.Call(ctx, "no_such_method", nil); !errors.As(err, &de) || de.Code != -32601 || de.Method != "no_such_method" {
		t.Errorf("device error %v", err)
	}
	dead := &miio.Client{Addr: "127.0.0.1:1", Token: make([]byte, 16), Timeout: 300 * time.Millisecond}
	if ok, err := miio.SupportsMIoT(ctx, dead); ok || err == nil {
		t.Errorf("no device: %v %v", ok, err)
	}
}
