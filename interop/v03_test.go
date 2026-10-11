// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package interop

import (
	"testing"
	"time"

	"github.com/cB-Sigma/miio-sim/sim"
)

// python-miio's RoborockVacuum extras against the simulated vacuum:
// consumables and a reset, the history and a record, do-not-disturb,
// timers (python-miio reads the time zone for them), volume, fan speed,
// carpet mode and find; then Wi-Fi setup and python-miio's MIoT probe on
// a legacy-only device.
func TestPythonMiioVacuumExtras(t *testing.T) {
	needDocker(t)
	v := startAtMiioPort(t, 0x0a1b2c62, sim.PlugToken, "roborock.vacuum.s5", map[string]any{"state": 8.0, "battery": 100.0})
	v.Configure(func(d *sim.Device) { d.Vacuum = sim.NewVacuumState() })
	p := startPeer(t, dockerArgs("vacuum_more", sim.PlugToken)...)
	get := func(step string) any { return event(t, p.expect(30*time.Second, step)) }
	ok := []any{"ok"}
	if c := get("consumables").(map[string]any); c["main"] != 36000.0 || c["filter"] != 18000.0 || c["sensor"] != 7200.0 {
		t.Errorf("consumables %v", c)
	}
	if r := get("reset"); !equalJSON(r, ok) {
		t.Errorf("reset %v", r)
	}
	if c := get("consumables2").(map[string]any); c["filter"] != 0.0 {
		t.Errorf("after reset %v", c)
	}
	if h := get("history").(map[string]any); h["count"] != 2.0 || h["seconds"] != 2940.0 || h["area"] != 53.75 || !equalJSON(h["ids"], []any{1760000000, 1759900000}) {
		t.Errorf("history %v", h)
	}
	if r := get("record").(map[string]any); r["start"] != 1760000000.0 || r["seconds"] != 2100.0 || r["area"] != 42.5 || r["complete"] != true {
		t.Errorf("record %v", r)
	}
	if d := get("dnd").(map[string]any); d["enabled"] != true || d["start"] != "22:00:00" || d["end"] != "07:00:00" {
		t.Errorf("dnd %v", d)
	}
	for _, step := range []string{"set_dnd", "disable_dnd", "add_timer", "update_timer"} {
		if r := get(step); !equalJSON(r, ok) {
			t.Errorf("%s %v", step, r)
		}
	}
	ts := get("timers").([]any)
	if len(ts) != 2 || !equalJSON(ts[1], map[string]any{"cron": "0 18 * * 6", "enabled": false, "id": "1770000000000"}) {
		t.Errorf("timers %v", ts)
	}
	if r := get("delete_timer"); !equalJSON(r, ok) {
		t.Errorf("delete_timer %v", r)
	}
	if r := get("volume"); r != 70.0 {
		t.Errorf("volume %v", r)
	}
	get("set_volume")
	if r := get("fan"); r != 60.0 {
		t.Errorf("fan %v", r)
	}
	get("set_fan")
	if c := get("carpet").(map[string]any); c["enabled"] != true || c["high"] != 500.0 {
		t.Errorf("carpet %v", c)
	}
	if r := get("set_carpet"); r != true {
		t.Errorf("set_carpet %v", r)
	}
	get("find")
	if r := get("miot"); r != false {
		t.Errorf("python-miio's MIoT probe on a legacy vacuum: %v", r)
	}
	if r := get("wifi"); r != "ok" {
		t.Errorf("configure_wifi %v", r)
	}
	p.expect(15*time.Second, "done")
	var st sim.VacuumState
	var wifi map[string]any
	v.Configure(func(d *sim.Device) { st, wifi = *d.Vacuum, d.WiFi })
	if st.DNDEnabled || st.DNDStartH != 23 || st.DNDEndM != 15 || st.Volume != 35 || st.FanSpeed != 102 || st.Carpet["enable"] != 0.0 ||
		st.Carpet["current_low"] != 400.0 || st.Found != 1 || len(st.Timers) != 1 || st.Consumables["filter_work_time"] != 0 {
		t.Errorf("simulator state after python-miio %+v", st)
	}
	if wifi["ssid"] != "site-net" || wifi["passwd"] != "demo-wifi-pass" || wifi["uid"] != 1234.0 {
		t.Errorf("config_router %v", wifi)
	}
}
