// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

// Command miiosim serves simulated Xiaomi Mi Home devices on 127.0.0.1,
// so a miIO client can be tried without devices:
//
//	chuangmi.plug.m3        plug, legacy methods
//	yeelink.light.color1    colour bulb, legacy methods
//	zhimi.airpurifier.ma4   air purifier, MIoT only
//
// Each device has its own UDP port (real devices use 54321 on their own
// addresses); the tokens are demo values printed at start. With -cloud it
// also serves a simulated Mi account (user demo@example.invalid, password
// -password) that lists the three devices with their tokens in region
// i2. For example:
//
//	miiosim -cloud
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/cB-Sigma/miio"
	"github.com/cB-Sigma/miio-sim/sim"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, nil); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "miiosim:", err)
		os.Exit(1)
	}
}

// demoUser is the simulated account's user name.
const demoUser = "demo@example.invalid"

type running struct {
	demo    *sim.Demo
	cloud   *sim.Cloud
	gateway *sim.Device
}

// run serves until ctx ends. ready, when not nil, receives what runs once
// it listens.
func run(ctx context.Context, args []string, out io.Writer, ready func(running)) error {
	fs := flag.NewFlagSet("miiosim", flag.ContinueOnError)
	fs.SetOutput(out)
	withCloud := fs.Bool("cloud", false, "also serve a simulated Mi account listing the devices")
	password := fs.String("password", "demo-mi-password", "password of the simulated Mi account")
	withGateway := fs.Bool("gateway", false, "also serve a lumi.gateway.v3 gateway with a weather and a door sensor")
	verify := fs.Bool("verify", false, "the simulated Mi account asks for a code at sign-in (the code is printed)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments %v", fs.Args())
	}
	d, err := sim.NewDemo()
	if err != nil {
		return err
	}
	defer d.Close()
	r := running{demo: d}
	devs := []struct {
		name, token string
		dev         *sim.Device
	}{{"plug", sim.PlugToken, d.Plug}, {"light", sim.LightToken, d.Light}, {"purifier", sim.PurifierToken, d.Purifier}}
	var list []miio.CloudDevice
	for _, x := range devs {
		fmt.Fprintf(out, "miiosim: %-8s %-22s did %d on udp %s token %s\n", x.name, x.dev.Model, x.dev.ID, x.dev.Addr(), x.token)
		host, _, _ := net.SplitHostPort(x.dev.Addr())
		list = append(list, miio.CloudDevice{DID: strconv.FormatUint(uint64(x.dev.ID), 10), Token: x.token, Name: x.name, Model: x.dev.Model, LocalIP: host, Online: true})
	}
	if *withGateway {
		gw, err := sim.Start(0x0a1b2c04, sim.GatewayToken, "lumi.gateway.v3", map[string]any{})
		if err != nil {
			return err
		}
		defer gw.Close()
		gw.Configure(func(g *sim.Device) { g.SubDevices = sim.DemoSubDevices() })
		r.gateway = gw
		did := strconv.FormatUint(uint64(gw.ID), 10)
		fmt.Fprintf(out, "miiosim: %-8s %-22s did %d on udp %s token %s\n", "gateway", gw.Model, gw.ID, gw.Addr(), sim.GatewayToken)
		host, _, _ := net.SplitHostPort(gw.Addr())
		list = append(list, miio.CloudDevice{DID: did, Token: sim.GatewayToken, Name: "gateway", Model: gw.Model, LocalIP: host, Online: true})
		for _, s := range sim.DemoSubDevices() {
			fmt.Fprintf(out, "miiosim:   sub-device %s %s\n", s.SID, s.Model)
			list = append(list, miio.CloudDevice{DID: s.SID, Name: s.Model, Model: s.Model, Online: true, ParentID: did, ParentModel: gw.Model})
		}
	}
	if *withCloud {
		r.cloud = sim.StartCloud(demoUser, *password, map[string][]miio.CloudDevice{"i2": list})
		defer r.cloud.Close()
		fmt.Fprintf(out, "miiosim: Mi account %s at %s (API %s)\n", demoUser, r.cloud.HTTP.URL, r.cloud.APIBase("i2"))
		if *verify {
			r.cloud.Verify = true
			fmt.Fprintf(out, "miiosim: the account asks for a code at sign-in; the code is 246810\n")
		}
	}
	if ready != nil {
		ready(r)
	}
	<-ctx.Done()
	return nil
}
