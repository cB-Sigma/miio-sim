// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package interop

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cB-Sigma/miio"
	"github.com/cB-Sigma/miio-sim/sim"
)

const (
	htSID   = "lumi.158d0001aaaa01"
	doorSID = "lumi.158d0001aaaa02"
)

func subs() []*sim.SubDevice {
	return []*sim.SubDevice{
		{SID: htSID, Model: "lumi.sensor_ht.v1", TypeID: 10, Firmware: 3, Battery: 87, Props: map[string]any{"temperature": 2315.0, "humidity": 4820.0}},
		{SID: doorSID, Model: "lumi.sensor_magnet.v2", TypeID: 3, Firmware: 5, Battery: 55, Props: map[string]any{}},
	}
}

func dockerArgs(mode string, extra ...string) []string {
	return append([]string{"--add-host=host.docker.internal:host-gateway", image, "python", "-u", "client.py", mode, "host.docker.internal", "54321"}, extra...)
}

// python-miio's Gateway against the simulated gateway: sub-device
// discovery from the device_list property, the weather sensor's readings
// through get_device_prop_exp, its battery, a property written.
func TestPythonMiioGateway(t *testing.T) {
	needDocker(t)
	gw := startAtMiioPort(t, 0x0a1b2c30, sim.PlugToken, "lumi.gateway.v3", map[string]any{})
	gw.Configure(func(d *sim.Device) { d.SubDevices = subs() })
	p := startPeer(t, dockerArgs("gateway", sim.PlugToken, htSID)...)
	devs := event(t, p.expect(60*time.Second, "devices")).(map[string]any)
	ht, _ := devs[htSID].(map[string]any)
	door, _ := devs[doorSID].(map[string]any)
	if len(devs) != 2 || ht["zigbee"] != "lumi.sensor_ht.v1" || ht["fw"] != 3.0 || door["zigbee"] != "lumi.sensor_magnet.v2" {
		t.Errorf("python-miio found %v", devs)
	}
	if st := event(t, p.expect(15*time.Second, "ht")).(map[string]any); st["temperature"] != 23.15 || st["humidity"] != 48.2 {
		t.Errorf("readings %v", st)
	}
	if b := event(t, p.expect(15*time.Second, "battery")); b != 87.0 {
		t.Errorf("battery %v", b)
	}
	if s := event(t, p.expect(15*time.Second, "set")); !equalJSON(s, []any{"ok"}) || gw.SubProp(htSID, "alias") != "kitchen" {
		t.Errorf("set %v", s)
	}
	p.expect(15*time.Second, "done")
}

// python-miio's firmware update (Device.update, update_state,
// update_progress) against the simulated plug, which downloads the image
// from a test server and checks its MD5.
func TestPythonMiioFirmwareUpdate(t *testing.T) {
	needDocker(t)
	image := []byte("simulated firmware image")
	sum := md5.Sum(image)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(image) }))
	defer srv.Close()
	d := startAtMiioPort(t, 0x0a1b2c40, sim.PlugToken, "chuangmi.plug.m3", map[string]any{"power": "on"})
	d.Configure(func(x *sim.Device) { x.OTAVersion, x.OTAStep = "9.9.9_0001", 100*time.Millisecond })
	p := startPeer(t, dockerArgs("ota", sim.PlugToken, srv.URL+"/fw.bin", hex.EncodeToString(sum[:]))...)
	if ok := event(t, p.expect(60*time.Second, "update")); ok != true {
		t.Errorf("update %v", ok)
	}
	st := event(t, p.expect(30*time.Second, "states")).(map[string]any)
	if fmt.Sprint(st["states"]) != "[downloading installing idle]" || st["progress"] != 100.0 || d.FirmwareVersion() != "9.9.9_0001" {
		t.Errorf("states %v, version %s", st, d.FirmwareVersion())
	}
	p.expect(15*time.Second, "done")
}

// python-miio's RoborockVacuum and ChuangmiPlug against the legacy
// simulators.
func TestPythonMiioVacuumAndPlug(t *testing.T) {
	needDocker(t)
	v := startAtMiioPort(t, 0x0a1b2c60, sim.PlugToken, "roborock.vacuum.s5", map[string]any{"state": 8.0, "battery": 100.0})
	p := startPeer(t, dockerArgs("vacuum", sim.PlugToken)...)
	if r := event(t, p.expect(60*time.Second, "start")); !equalJSON(r, []any{"ok"}) {
		t.Errorf("start %v", r)
	}
	if st := event(t, p.expect(15*time.Second, "status")).(map[string]any); st["state"] != 5.0 || st["battery"] != 100.0 {
		t.Errorf("status %v", st)
	}
	p.expect(15*time.Second, "home")
	p.expect(15*time.Second, "done")
	if v.Prop("state") != 6.0 {
		t.Errorf("state after home %v", v.Prop("state"))
	}
	v.Close()

	pl := startAtMiioPort(t, 0x0a1b2c61, sim.PlugToken, "chuangmi.plug.v1", map[string]any{"on": false, "temperature": 31.0})
	q := startPeer(t, dockerArgs("plug", sim.PlugToken)...)
	q.expect(60*time.Second, "on")
	if st := event(t, q.expect(15*time.Second, "status")).(map[string]any); st["on"] != true || st["temperature"] != 31.0 || pl.Prop("on") != true {
		t.Errorf("plug %v", st)
	}
	q.expect(15*time.Second, "done")
}

// micloud 0.6 (the Mi account client python-miio depends on) against the
// simulated account: sign-in with its form-encoded steps, the older
// home/device_list, and a property read through miotspec/prop/get from a
// linked device.
func TestMicloudAgainstAccount(t *testing.T) {
	needDocker(t)
	pur, err := sim.Start(0x0a1b2c03, sim.PurifierToken, "zhimi.airpurifier.ma4", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	defer pur.Close()
	pur.Configure(func(p *sim.Device) { p.MIoTOnly, p.MIoT = true, map[string]any{"3.6": 42.0} })
	sc := sim.StartCloud("user@example.invalid", "pw", map[string][]miio.CloudDevice{
		"i2": {{DID: "300", Token: sim.PurifierToken, Model: "zhimi.airpurifier.ma4", LocalIP: "192.168.1.70"}}})
	defer sc.Close()
	sc.Link("300", pur)
	base := strings.Replace(sc.HTTP.URL, "127.0.0.1", "host.docker.internal", 1)
	p := startPeer(t, "--add-host=host.docker.internal:host-gateway", image, "python", "-u", "client.py", "micloud", base, "user@example.invalid", "pw", "300")
	if ok := event(t, p.expect(60*time.Second, "login")); ok != true {
		t.Fatalf("micloud login %v", ok)
	}
	devs := event(t, p.expect(15*time.Second, "devices")).([]any)
	if len(devs) != 1 || devs[0].(map[string]any)["token"] != sim.PurifierToken {
		t.Errorf("micloud devices %v", devs)
	}
	prop := event(t, p.expect(15*time.Second, "prop")).(map[string]any)
	res, _ := prop["result"].([]any)
	if prop["code"] != 0.0 || len(res) != 1 || res[0].(map[string]any)["value"] != 42.0 {
		t.Errorf("micloud prop/get %v", prop)
	}
	p.expect(15*time.Second, "done")
	if sc.Logins() != 1 {
		t.Errorf("logins %d", sc.Logins())
	}
}

// python-miio's PushServer subscribes to a sub-device event on the
// simulated gateway (send_data_frame), the gateway pushes the event to it
// when it happens, and the scene is removed with miIO.xdel.
func TestPythonMiioPushServer(t *testing.T) {
	needDocker(t)
	gw := startAtMiioPort(t, 0x0a1b2c50, sim.PlugToken, "lumi.gateway.v3", map[string]any{})
	gw.Configure(func(d *sim.Device) { d.SubDevices = subs() })
	hp := freePort(t)
	p := startPeer(t, append([]string{"-p", fmt.Sprintf("127.0.0.1:%d:54321/udp", hp)}, dockerArgs("push", sim.PlugToken, doorSID)...)...)
	sub := event(t, p.expect(60*time.Second, "subscribed")).(map[string]any)
	if len(gw.Scenes()) != 1 || gw.Scenes()[0] != sub["id"] {
		t.Fatalf("scenes %v, python-miio %v", gw.Scenes(), sub)
	}
	gw.Configure(func(d *sim.Device) { d.PushTo = fmt.Sprintf("127.0.0.1:%d", hp) })
	if n, err := gw.Emit(doorSID, "open", []any{"open"}); err != nil || n != 1 {
		t.Fatalf("emit %d %v", n, err)
	}
	ev := event(t, p.expect(15*time.Second, "event")).(map[string]any)
	if ev["source"] != doorSID || !equalJSON(ev["params"], []any{"open"}) {
		t.Errorf("python-miio received %v", ev)
	}
	p.send("unsubscribe")
	p.expect(15*time.Second, "unsubscribed")
	if len(gw.Scenes()) != 0 {
		t.Errorf("scenes after miIO.xdel %v", gw.Scenes())
	}
	p.expect(15*time.Second, "done")
}
