// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package miio_test

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/cB-Sigma/miio"
	"github.com/cB-Sigma/miio-sim/sim"
)

// List the simulated account's devices, then switch one on its LAN.
func Example() {
	plug, _ := sim.Start(123456789, sim.PlugToken, "chuangmi.plug.m3", map[string]any{"power": "off"})
	defer plug.Close()
	account := sim.StartCloud("user@example.invalid", "secret", map[string][]miio.CloudDevice{
		"i2": {{DID: "123456789", Token: sim.PlugToken, Model: "chuangmi.plug.m3", LocalIP: "127.0.0.1"}}})
	defer account.Close()
	ctx := context.Background()

	cloud := &miio.Cloud{Username: "user@example.invalid", Password: "secret",
		AccountBase: account.HTTP.URL, APIBase: account.APIBase, HTTP: account.HTTP.Client()} // leave these three empty for Xiaomi's servers
	devs, _ := cloud.Devices(ctx, []string{"i2"})
	tok, _ := hex.DecodeString(devs[0].Token)

	c := &miio.Client{Addr: plug.Addr(), Token: tok, Timeout: 3 * time.Second} // a real device: <localip>:54321
	_, err := c.Call(ctx, "set_power", []any{"on"})
	fmt.Println(devs[0].Model, err, plug.Prop("power"))
	// Output: chuangmi.plug.m3 <nil> on
}

// Read a simulated MIoT device's properties by service and property ID.
func ExampleGetProperties() {
	demo, _ := sim.NewDemo()
	defer demo.Close()
	tok, _ := hex.DecodeString(sim.PurifierToken)
	c := &miio.Client{Addr: demo.Purifier.Addr(), Token: tok, Timeout: 3 * time.Second}
	res, _ := miio.GetProperties(context.Background(), c, []miio.PropRef{
		{DID: "purifier", SIID: 2, PIID: 2}, // power
		{DID: "purifier", SIID: 3, PIID: 6}, // PM2.5
	})
	for _, r := range res {
		fmt.Println(r.SIID, r.PIID, r.Code, r.Value)
	}
	// Output:
	// 2 2 0 true
	// 3 6 0 42
}
