// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package sim

import (
	"fmt"
	"strings"
)

// VacuumState is what a simulated Roborock vacuum keeps beyond its
// status: the consumable counters, the cleaning history, the
// do-not-disturb period, the timers, the voice volume, the suction
// setting, carpet detection and its time zone. All values are made-up
// demo values.
type VacuumState struct {
	// Consumables are counters by their get_consumable keys (seconds of
	// use, or a count for the *_times keys).
	Consumables map[string]float64
	// Records are the stored cleaning runs, newest first.
	Records []VacuumRecord
	// DND: enabled and the period.
	DNDEnabled                             bool
	DNDStartH, DNDStartM, DNDEndH, DNDEndM int
	Timers                                 []VacuumTimer
	Volume, FanSpeed                       int
	Carpet                                 map[string]any // enable, stall_time, current_low, current_high, current_integral
	Timezone                               string
	// ObjectAnswers answers get_clean_summary and get_clean_record with
	// objects, as newer firmware does; otherwise with lists, as older
	// firmware does.
	ObjectAnswers bool
	// Found counts find_me requests; VolumeTests test_sound_volume ones.
	Found, VolumeTests int
}

// VacuumRecord is one stored cleaning run (times in Unix seconds, area in
// mm²).
type VacuumRecord struct {
	Begin, End, Duration, Area, Error int64
	Complete                          bool
}

// VacuumTimer is one stored timer.
type VacuumTimer struct {
	ID, Cron string
	On       bool
}

// NewVacuumState returns the demo values.
func NewVacuumState() *VacuumState {
	return &VacuumState{
		Consumables: map[string]float64{"main_brush_work_time": 36000, "side_brush_work_time": 36000, "filter_work_time": 18000, "sensor_dirty_time": 7200},
		Records: []VacuumRecord{
			{Begin: 1760000000, End: 1760002400, Duration: 2100, Area: 42500000, Complete: true},
			{Begin: 1759900000, End: 1759900900, Duration: 840, Area: 11250000, Error: 3},
		},
		DNDEnabled: true, DNDStartH: 22, DNDEndH: 7,
		Timers:   []VacuumTimer{{ID: "1760000000123", Cron: "30 9 * * 1,3,5", On: true}},
		Volume:   70,
		FanSpeed: 60,
		Carpet:   map[string]any{"enable": 1.0, "stall_time": 10.0, "current_low": 400.0, "current_high": 500.0, "current_integral": 450.0},
		Timezone: "Asia/Kolkata",
	}
}

func intArg(v any) (int, bool) {
	f, ok := v.(float64)
	return int(f), ok && f == float64(int(f))
}

// vacuumMore answers the Roborock methods beyond start and stop. The
// caller holds d.mu.
func (d *Device) vacuumMore(method string, p []any) (any, bool) {
	if d.Vacuum == nil {
		d.Vacuum = NewVacuumState()
	}
	v := d.Vacuum
	arg := func(i int) any {
		if i < len(p) {
			return p[i]
		}
		return nil
	}
	ok := []any{"ok"}
	bad := devError{-10000, "invalid parameter"}
	switch method {
	case "get_consumable":
		out := map[string]any{}
		for k, n := range v.Consumables {
			out[k] = n
		}
		return []any{out}, true
	case "reset_consumable":
		k, _ := arg(0).(string)
		if _, known := v.Consumables[k]; !known {
			return bad, true
		}
		v.Consumables[k] = 0
	case "get_clean_summary":
		var total, area int64
		ids := []any{}
		for _, r := range v.Records {
			total, area = total+r.Duration, area+r.Area
			ids = append(ids, r.Begin)
		}
		if v.ObjectAnswers {
			return map[string]any{"clean_time": total, "clean_area": area, "clean_count": len(v.Records), "records": ids}, true
		}
		return []any{total, area, len(v.Records), ids}, true
	case "get_clean_record":
		id, isInt := intArg(arg(0))
		if !isInt {
			return bad, true
		}
		for _, r := range v.Records {
			if r.Begin != int64(id) {
				continue
			}
			c := 0
			if r.Complete {
				c = 1
			}
			if v.ObjectAnswers {
				return []any{map[string]any{"begin": r.Begin, "end": r.End, "duration": r.Duration, "area": r.Area, "error": r.Error, "complete": c}}, true
			}
			return []any{[]any{r.Begin, r.End, r.Duration, r.Area, r.Error, c}}, true
		}
		return []any{}, true
	case "get_dnd_timer":
		en := 0
		if v.DNDEnabled {
			en = 1
		}
		return []any{map[string]any{"enabled": en, "start_hour": v.DNDStartH, "start_minute": v.DNDStartM, "end_hour": v.DNDEndH, "end_minute": v.DNDEndM}}, true
	case "set_dnd_timer":
		var n [4]int
		for i := range n {
			x, isInt := intArg(arg(i))
			if !isInt || x < 0 || (i%2 == 0 && x > 23) || (i%2 == 1 && x > 59) {
				return bad, true
			}
			n[i] = x
		}
		v.DNDEnabled, v.DNDStartH, v.DNDStartM, v.DNDEndH, v.DNDEndM = true, n[0], n[1], n[2], n[3]
	case "close_dnd_timer":
		v.DNDEnabled = false
	case "get_timer":
		out := []any{}
		for _, t := range v.Timers {
			st := "off"
			if t.On {
				st = "on"
			}
			out = append(out, []any{t.ID, st, []any{t.Cron, []any{"start_clean", ""}}})
		}
		return out, true
	case "set_timer":
		rec, _ := arg(0).([]any)
		if len(rec) < 2 {
			return bad, true
		}
		id, _ := rec[0].(string)
		sched, _ := rec[1].([]any)
		if id == "" || len(sched) < 1 {
			return bad, true
		}
		cron, _ := sched[0].(string)
		if len(strings.Fields(cron)) != 5 {
			return bad, true
		}
		v.Timers = append(v.Timers, VacuumTimer{ID: id, Cron: cron, On: true})
	case "upd_timer", "del_timer":
		id, _ := arg(0).(string)
		for i, t := range v.Timers {
			if t.ID != id {
				continue
			}
			if method == "del_timer" {
				v.Timers = append(v.Timers[:i], v.Timers[i+1:]...)
				return ok, true
			}
			switch arg(1) {
			case "on":
				v.Timers[i].On = true
			case "off":
				v.Timers[i].On = false
			default:
				return bad, true
			}
			return ok, true
		}
		return bad, true
	case "get_timezone":
		return []any{v.Timezone}, true
	case "get_sound_volume":
		return []any{v.Volume}, true
	case "change_sound_volume":
		n, isInt := intArg(arg(0))
		if !isInt || n < 0 || n > 100 {
			return bad, true
		}
		v.Volume = n
	case "test_sound_volume":
		v.VolumeTests++
	case "get_custom_mode":
		return []any{v.FanSpeed}, true
	case "set_custom_mode":
		n, isInt := intArg(arg(0))
		if !isInt || n < 0 {
			return bad, true
		}
		v.FanSpeed = n
	case "get_carpet_mode":
		out := map[string]any{}
		for k, x := range v.Carpet {
			out[k] = x
		}
		return []any{out}, true
	case "set_carpet_mode":
		m, _ := arg(0).(map[string]any)
		for _, k := range []string{"enable", "stall_time", "current_low", "current_high", "current_integral"} {
			if _, isInt := intArg(m[k]); !isInt {
				return devError{-10000, fmt.Sprintf("%s is missing", k)}, true
			}
		}
		v.Carpet = m
	case "find_me":
		v.Found++
	default:
		return nil, false
	}
	return ok, true
}
