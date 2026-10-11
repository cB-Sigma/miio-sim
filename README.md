# miio-sim

Simulated Xiaomi Mi Home devices on miIO and MIoT, a simulated Mi account and a MIoT specification service, for testing miIO clients without devices; not affiliated with Xiaomi.

[![Go Reference](https://pkg.go.dev/badge/github.com/cB-Sigma/miio-sim.svg)](https://pkg.go.dev/github.com/cB-Sigma/miio-sim) [![test](https://github.com/cB-Sigma/miio-sim/actions/workflows/test.yml/badge.svg)](https://github.com/cB-Sigma/miio-sim/actions/workflows/test.yml) [![Licence: Apache-2.0](https://img.shields.io/badge/licence-Apache--2.0-blue.svg)](LICENSE) [![Go](https://img.shields.io/badge/go-1.22-00ADD8.svg)](go.mod)

Each simulated device answers the hello handshake and token-encrypted
JSON-RPC on its own UDP port: miIO.info, get_prop and the setters of
plugs, lights and air purifiers, and the MIoT get_properties,
set_properties and action requests. The simulated Mi account signs in
and lists devices with their tokens; the specification service serves
MIoT specifications by model. It is built on
[miio](https://github.com/cB-Sigma/miio). Written for SigmaM2M, a oneM2M
platform; it has no dependency on it.

## What is simulated

| Feature | Status |
|---|---|
| Hello handshake, stamps, checksummed and encrypted packets; silence for a wrong token | yes |
| Legacy methods: miIO.info, get_prop, set_power, set_bright, set_ct_abx, set_rgb, set_mode, set_level_favorite | yes |
| Model behaviour: a Yeelight refuses changes while off; Philips lights take one argument per method and set_cct; chuangmi.plug.v1 switches with set_on/set_off; MIoT-only models refuse legacy methods | yes |
| MIoT get_properties, set_properties (codes -4002 read-only, -4003 unknown, -4005 wrong type), actions | yes |
| Devices that drop a request ID they already answered (`DropSeenIDs`) | yes |
| Mi account sign-in, homes and device lists per region, a session that ends, a verification it cannot pass (`Verify`) | yes |
| MIoT specification service (`StartSpecServer`) | yes |
| Gateways and their sub-devices, pushes, firmware update | no |

## Install

```
go get github.com/cB-Sigma/miio-sim@latest
```

Requires Go 1.22 or later. The command:

```
go install github.com/cB-Sigma/miio-sim/cmd/miiosim@latest
```

## Command line

```
miiosim -cloud
```

| Flag | Default | Meaning |
|---|---|---|
| `-cloud` | off | also serve a simulated Mi account (demo@example.invalid) that lists the devices with their tokens in region i2 |
| `-password` | `demo-mi-password` | password of the simulated account |

It serves a chuangmi.plug.m3 plug and a yeelink.light.color1 bulb on the
legacy methods and a zhimi.airpurifier.ma4 purifier on MIoT only, each on
its own port of 127.0.0.1, and prints their addresses and demo tokens.

## In Go tests

From `Example` in example_test.go:

```go
plug, _ := sim.Start(123456789, sim.PlugToken, "chuangmi.plug.m3", map[string]any{"power": "off"})
defer plug.Close()
account := sim.StartCloud("user@example.invalid", "secret", map[string][]miio.CloudDevice{
	"i2": {{DID: "123456789", Token: sim.PlugToken, Model: "chuangmi.plug.m3", LocalIP: "127.0.0.1"}}})
defer account.Close()

cloud := &miio.Cloud{Username: "user@example.invalid", Password: "secret",
	AccountBase: account.HTTP.URL, APIBase: account.APIBase, HTTP: account.HTTP.Client()}
devs, _ := cloud.Devices(ctx, []string{"i2"})
tok, _ := hex.DecodeString(devs[0].Token)
c := &miio.Client{Addr: plug.Addr(), Token: tok, Timeout: 3 * time.Second}
_, err := c.Call(ctx, "set_power", []any{"on"})
// plug.Prop("power") is now "on"
```

`StartAt("127.0.0.1:54321", ...)` puts a device on a given address, for
clients that send their hello only to the miIO port.

## Documentation

Reference: https://pkg.go.dev/github.com/cB-Sigma/miio-sim

| Package | What it does |
|---|---|
| `sim` | `Start`, `StartAt`, `Device` (`Prop`, `MIoTValue`, `Calls`, `Configure` for `DropSeenIDs`, `MIoTOnly`, `MIoT`, `ReadOnly`, `Actions`), `NewDemo`; `StartCloud` (`Cloud`: `APIBase`, `Logins`, `EndSession`, `Verify`); `StartSpecServer` |
| `cmd/miiosim` | Serves three demo devices and, with `-cloud`, an account listing them |
| `interop` (nested module, tests only) | python-miio's client against the simulators, in a container |

Changes per version are in [CHANGELOG.md](CHANGELOG.md).

## Testing

```
go test -race ./...
cd interop && SIGMA_MIIO_INTEROP=1 go test -v ./...   # needs Docker
```

The tests at the module root drive the miio client against the
simulators: the legacy client and discovery, MIoT, the account and the
specification service. Tests carry `// Covers: SIG-...` comments:
requirement IDs of the SigmaM2M project's compliance report, with no
meaning outside it.

### Interop tested against

- python-miio's client (master 9a00e08, python:3.12-slim in Docker
  Desktop on arm64, 2026-10-11; `interop`), with the simulated device on
  127.0.0.1:54321 because python-miio sends its hello only to that port:
  the hello handshake (python-miio read our device ID), `Device.info()`
  (model and MAC), get_prop, set_power and the changed value; a wrong
  token was refused; on the purifier, get_properties with values and the
  -4003 code, set_properties with a 0 and a -4002 code, and an action
  that ran. Passed; no simulator behaviour had to change.
- Not yet: compared with a real Xiaomi device or a real Mi account. The
  simulators and the miio client are written by the same author, so
  their agreement is not independent; the python-miio runs are.

## Demo values

Every token, device ID, account, password, MAC and address in this
module is a made-up demo value. Do not use them for real devices.

## Related repositories

- [miio](https://github.com/cB-Sigma/miio): the miIO, MIoT and Mi
  account client this module is built on.

## Status

v0.x: the API may change before v1.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## Security

See [SECURITY.md](SECURITY.md).

## Licence

Apache-2.0 (see [LICENSE](LICENSE) and [NOTICE](NOTICE)). Copyright 2026
Abhinav Gautam. Xiaomi, Mi, Mi Home, MIoT and Yeelight are trademarks of
their owners; the names are used only to say which devices and services
this module simulates.
