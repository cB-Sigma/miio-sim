// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package sim

import (
	"context"
	"net"
	"strconv"
	"strings"

	"github.com/cB-Sigma/miio"
)

// MDNSInstance is the instance name the device would announce as
// "_miio._udp": the model with dashes for dots, "_miio" and the device
// ID in decimal.
func (d *Device) MDNSInstance() string {
	return strings.ReplaceAll(d.Model, ".", "-") + "_miio" + strconv.FormatUint(uint64(d.ID), 10)
}

// Browser is a miio.Browser that answers with the given devices, each at
// its own loopback address and port, as an mDNS browser would find them.
// It sends nothing on the network: the simulator does not announce by
// mDNS.
func Browser(devs ...*Device) miio.Browser {
	return miio.BrowseFunc(func(ctx context.Context, service string) ([]miio.MDNSEntry, error) {
		if service != miio.MDNSService {
			return nil, nil
		}
		var out []miio.MDNSEntry
		for _, d := range devs {
			host, port, err := net.SplitHostPort(d.Addr())
			if err != nil {
				continue
			}
			n, _ := strconv.Atoi(port)
			out = append(out, miio.MDNSEntry{Instance: d.MDNSInstance() + "." + miio.MDNSService + ".local.", IPs: []net.IP{net.ParseIP(host)}, Port: n})
		}
		return out, ctx.Err()
	})
}
