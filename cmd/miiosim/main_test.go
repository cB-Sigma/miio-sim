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

// -gateway serves a gateway with two sub-devices, which the account lists
// under it; -verify makes the account ask for the printed code.
func TestRunnerGatewayVerify(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan running, 1)
	var out bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, []string{"-cloud", "-gateway", "-verify", "-password", "pw"}, &out, func(r running) { got <- r })
	}()
	var r running
	select {
	case r = <-got:
	case err := <-done:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("not ready")
	}
	c := &miio.Cloud{Username: demoUser, Password: "pw", AccountBase: r.cloud.HTTP.URL, APIBase: r.cloud.APIBase, HTTP: r.cloud.HTTP.Client()}
	if err := c.Login(ctx); err == nil {
		t.Fatal("no verification asked")
	}
	if err := c.SendCode(ctx, miio.ByEmail); err != nil {
		t.Fatal(err)
	}
	if err := c.VerifyCode(ctx, miio.ByEmail, "246810"); err != nil {
		t.Fatal(err)
	}
	devs, err := c.Devices(ctx, []string{"i2"})
	gwDID := ""
	for _, d := range devs {
		if d.Model == "lumi.gateway.v3" {
			gwDID = d.DID
		}
	}
	if err != nil || len(devs) != 6 || len(miio.GatewaySubDevices(devs, gwDID)) != 2 {
		t.Fatalf("account devices %v %v", devs, err)
	}
	tok, _ := hex.DecodeString("a0b1c2d3e4f5061728394a5b6c7d8e9f")
	g := &miio.Gateway{Client: &miio.Client{Addr: r.gateway.Addr(), Token: tok, Timeout: 2 * time.Second}, Model: "lumi.gateway.v3"}
	if subs, err := g.SubDevices(ctx); err != nil || len(subs) != 2 {
		t.Errorf("sub-devices %v %v", subs, err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// -vacuum serves a Roborock vacuum with its extras; -specs serves a
// mirror directory.
func TestRunnerVacuumSpecs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	urn := "urn:miot-spec-v2:device:outlet:0000A002:demo-plug:1"
	if err := (&miio.SpecMirror{Dir: dir}).Add("demo.plug.v1", 1, []byte(`{"type":"`+urn+`","services":[{"iid":2,"type":"urn:miot-spec-v2:service:switch:0000780C:demo-plug:1"}]}`)); err != nil {
		t.Fatal(err)
	}
	got := make(chan running, 1)
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, []string{"-vacuum", "-specs", dir}, &bytes.Buffer{}, func(r running) { got <- r })
	}()
	var r running
	select {
	case r = <-got:
	case err := <-done:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("not ready")
	}
	tok, _ := hex.DecodeString(vacuumToken)
	v := &miio.Vacuum{Family: miio.Family{Client: &miio.Client{Addr: r.vacuum.Addr(), Token: tok, Timeout: 2 * time.Second}}}
	if c, err := v.Consumables(ctx); err != nil || c.MainBrush == 0 {
		t.Errorf("consumables %+v %v", c, err)
	}
	if sp, err := (&miio.HTTPSpecs{Base: r.specs}).Spec(ctx, "demo.plug.v1"); err != nil || sp.Type != urn {
		t.Errorf("mirror %+v %v", sp, err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestRunnerFlags(t *testing.T) {
	for _, args := range [][]string{{"-bogus"}, {"extra"}} {
		if err := run(context.Background(), args, &bytes.Buffer{}, nil); err == nil {
			t.Errorf("%v accepted", args)
		}
	}
}
