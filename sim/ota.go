// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package sim

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"time"
)

// ota answers firmware update requests: miIO.ota with app_url and
// file_md5 starts a download (refused while one runs), which really
// fetches the image, checks its MD5 and, after an install phase, reports
// OTAVersion in miIO.info; miIO.get_ota_progress and miIO.get_ota_state
// report it. A failed download or a wrong MD5 leaves the state "failed".
// The caller holds d.mu.
func (d *Device) ota(method string, p []any) any {
	state := d.otaState
	if state == "" {
		state = "idle"
	}
	switch method {
	case "miIO.get_ota_progress":
		return []any{d.otaProgress}
	case "miIO.get_ota_state":
		return []any{state}
	}
	if state == "downloading" || state == "installing" {
		return devError{-5001, "busy"}
	}
	if len(p) != 1 {
		return devError{-5001, "invalid_arg"}
	}
	m, _ := p[0].(map[string]any)
	u, sum := toString(m["app_url"]), strings.ToLower(toString(m["file_md5"]))
	if u == "" || len(sum) != 32 {
		return devError{-5001, "invalid_arg"}
	}
	d.otaState, d.otaProgress = "downloading", 0
	step, hc := d.OTAStep, d.HTTP
	if step == 0 {
		step = 50 * time.Millisecond
	}
	if hc == nil {
		hc = http.DefaultClient
	}
	go d.runOTA(hc, u, sum, step)
	return []any{"ok"}
}

func (d *Device) setOTA(state string, progress int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.otaState, d.otaProgress = state, progress
}

func (d *Device) runOTA(hc *http.Client, u, sum string, step time.Duration) {
	time.Sleep(step)
	resp, err := hc.Get(u)
	if err != nil {
		d.setOTA("failed", 0)
		return
	}
	defer resp.Body.Close()
	h := md5.New()
	_, err = io.Copy(h, io.LimitReader(resp.Body, 64<<20))
	if err != nil || resp.StatusCode != http.StatusOK {
		d.setOTA("failed", 0)
		return
	}
	d.setOTA("downloading", 50)
	time.Sleep(step)
	if hex.EncodeToString(h.Sum(nil)) != sum {
		d.setOTA("failed", 100)
		return
	}
	d.setOTA("installing", 100)
	time.Sleep(step)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.otaState = "idle"
	d.Firmware = d.OTAVersion
	if d.Firmware == "" {
		d.Firmware = "2.1.10_0001"
	}
}

// FirmwareVersion is the version miIO.info reports.
func (d *Device) FirmwareVersion() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.Firmware == "" {
		return "2.1.9_0023"
	}
	return d.Firmware
}
