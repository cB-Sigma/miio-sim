// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/cB-Sigma/miio"
)

func TestRunner(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan running, 1)
	var out bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- run(ctx, []string{"-cloud", "-password", "pw"}, &out, func(r running) { got <- r }) }()
	var r running
	select {
	case r = <-got:
	case err := <-done:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("not ready")
	}
	c := &miio.Cloud{Username: demoUser, Password: "pw", AccountBase: r.cloud.HTTP.URL, APIBase: r.cloud.APIBase, HTTP: r.cloud.HTTP.Client()}
	devs, err := c.Devices(ctx, []string{"i2"})
	if err != nil || len(devs) != 3 {
		t.Fatalf("account devices %v %v", devs, err)
	}
	for _, d := range devs {
		if d.Model != "chuangmi.plug.m3" {
			continue
		}
		tok, _ := hex.DecodeString(d.Token)
		cl := &miio.Client{Addr: r.demo.Plug.Addr(), Token: tok, Timeout: 2 * time.Second}
		if _, err := cl.Call(ctx, "miIO.info", nil); err != nil {
			t.Errorf("plug with the account's token: %v", err)
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "zhimi.airpurifier.ma4") || !strings.Contains(out.String(), "Mi account") {
		t.Errorf("output %s", out.String())
	}
}

func TestRunnerFlags(t *testing.T) {
	for _, args := range [][]string{{"-bogus"}, {"extra"}} {
		if err := run(context.Background(), args, &bytes.Buffer{}, nil); err == nil {
			t.Errorf("%v accepted", args)
		}
	}
}
