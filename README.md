# miio-sim

Simulated Xiaomi Mi Home devices on miIO and MIoT, a simulated Mi account and a MIoT specification service, for testing miIO clients without devices; not affiliated with Xiaomi.

[![Go Reference](https://pkg.go.dev/badge/github.com/cB-Sigma/miio-sim.svg)](https://pkg.go.dev/github.com/cB-Sigma/miio-sim) [![test](https://github.com/cB-Sigma/miio-sim/actions/workflows/test.yml/badge.svg)](https://github.com/cB-Sigma/miio-sim/actions/workflows/test.yml) [![Licence: Apache-2.0](https://img.shields.io/badge/licence-Apache--2.0-blue.svg)](LICENSE) [![Go](https://img.shields.io/badge/go-1.22-00ADD8.svg)](go.mod)

Each simulated device answers the hello handshake and token-encrypted
JSON-RPC on its own UDP port: miIO.info, get_prop and the setters of
plugs, lights, air purifiers and Roborock vacuums, and the MIoT
get_properties, set_properties and action requests. Simulated gateways
serve Zigbee sub-devices; every device takes firmware updates (really
downloading the image and checking its MD5) and local push scenes, and
pushes events. The simulated Mi account signs in (with a captcha or an
identity check when asked), lists devices with their tokens and relays
device control; the specification service serves MIoT specifications by
model. It is built on
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
| Roborock vacuums (roborock.vacuum.*, rockrobo.vacuum.*): app_start, app_stop, app_pause, app_charge, app_spot, get_status | yes |
| Roborock extras (`Device.Vacuum`): consumables and their reset, clean summary and records (list or object answers), do-not-disturb, timers, time zone, sound volume, custom mode, carpet mode, find_me | yes |
| Roborock maps, rooms, zones, mop and dock methods | no |
| Wi-Fi setup (miIO.config_router, kept in `Device.WiFi`) | yes |
| mDNS names (`MDNSInstance`) and a `Browser` listing simulated devices for miio's `DiscoverMDNS` | yes; nothing is announced on the network |
| lumi gateways and their Zigbee sub-devices (`SubDevices`): device_list (older gateways), get_device_list (lumi.gateway.mgl03), get_device_prop, get_device_prop_exp, set_device_prop, get_battery, remove_device | yes |
| Firmware update: miIO.ota downloads the image from its URL and checks the MD5, then miIO.get_ota_progress and miIO.get_ota_state follow it to the new `OTAVersion` or "failed" | yes |
| Pushes: send_data_frame scenes, miIO.xdel, `Emit` sending the event to the scene's target and waiting for its answer (`PushTo` for a test listener) | yes |
| Mi account sign-in, homes and device lists per region (and the older home/device_list), form or query parameters, a session that ends | yes |
| Mi account captcha (`Captcha`) and identity check by e-mail or SMS (`Verify`), completed with the demo text and code | yes |
| Device control through the account for linked devices (`Link`): miotspec/prop/get, prop/set, action, home/rpc | yes |
| MIoT specification service (`StartSpecServer`): index (released, or all with statuses), specifications, translations (`SetTranslations`), an outage (`SetDown`) for testing miio's `SpecMirror` | yes |
| Bluetooth sub-devices of gateways | no: no local method is known to simulate; a Bluetooth device's readings can be served through `Link` and the account |

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
| `-gateway` | off | also serve a lumi.gateway.v3 gateway with a weather sensor and a door sensor (listed under it in the account) |
| `-verify` | off | the account asks for a code at sign-in; the code is printed |
| `-vacuum` | off | also serve a roborock.vacuum.s5 vacuum on the legacy methods, with the extras |
| `-specs` | none | also serve this MIoT specification mirror directory (as miio's `miiospec` fills it), offline |

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
| `sim` | `Start`, `StartAt`, `Device` (`Prop`, `MIoTValue`, `Calls`, `Configure` for `DropSeenIDs`, `MIoTOnly`, `MIoT`, `ReadOnly`, `Actions`, `SubDevices`, `Firmware`, `OTAVersion`, `OTAStep`, `HTTP`, `PushTo`, `Vacuum`, `WiFi`; `SubProp`, `FirmwareVersion`, `Scenes`, `Emit`, `MDNSInstance`), `VacuumState`, `NewVacuumState`, `VacuumRecord`, `VacuumTimer`, `Browser`, `SubDevice`, `DemoSubDevices`, `NewDemo`; `StartCloud` (`Cloud`: `APIBase`, `Logins`, `EndSession`, `Verify`, `Captcha`, `Code`, `CaptchaText`, `CodesSent`, `Verified`, `Link`); `StartSpecServer` (`SetTranslations`, `SetDown`) |
| `cmd/miiosim` | Serves three demo devices and, with `-gateway`, a gateway with two sub-devices, with `-vacuum` a Roborock vacuum; with `-cloud`, an account listing them; with `-specs`, a specification mirror |
| `interop` (nested module, tests only) | python-miio's client and micloud against the simulators, in a container |

Changes per version are in [CHANGELOG.md](CHANGELOG.md).

## Testing

```
go test -race ./...
cd interop && SIGMA_MIIO_INTEROP=1 go test -v ./...   # needs Docker
```

The tests at the module root drive the miio client against the
simulators: the legacy client and discovery, MIoT, the specification
mapping and family helpers, gateways, firmware update, pushes, the
account with its verification steps and device control, the
specification service and a specification mirror filled from it, the
vacuum extras, mDNS discovery, Wi-Fi setup and the MIoT probe
(v03_test.go). Tests carry `// Covers: SIG-...` comments: requirement
IDs of the SigmaM2M project's compliance report, with no meaning
outside it; the gateway, firmware, push and v0.3.0 tests claim none.

### Interop tested against

v0.3.0: python-miio master 9a00e08 (0.6.0.dev0) with micloud 0.6, on
python:3.12.10-slim, Docker Desktop on arm64, 2026-10-12 (`interop`; all
eight tests passed, the seven earlier ones repeated):

- python-miio's `RoborockVacuum` against the simulated vacuum with
  `NewVacuumState`: consumable_status, consumable_reset, clean_history
  and clean_details, dnd_status, set_dnd, disable_dnd, add_timer,
  update_timer, timer (which reads the time zone), delete_timer,
  sound_volume, set_sound_volume, fan_speed, set_fan_speed,
  carpet_mode, set_carpet_mode and find all read or changed what the
  simulator holds; python-miio's supports_miot said no for the legacy
  vacuum, and configure_wifi reached miIO.config_router with ssid,
  passwd and uid.
- The specification mirror is checked against python-miio's own client
  in the miio module's `interop`; here it runs against the simulated
  service (module root tests).

Earlier runs:

All on python:3.12.10-slim with python-miio master 9a00e08 (installs as
0.6.0.dev0) and the micloud 0.6 it depends on, Docker Desktop on arm64,
2026-10-11 (`interop`; all passed). Devices sit on 127.0.0.1:54321
because python-miio sends its hello only to that port.

- python-miio's client: the hello handshake (python-miio read our device
  ID), `Device.info()` (model and MAC), get_prop, set_power and the
  changed value; a wrong token was refused; on the purifier,
  get_properties with values and the -4003 code, set_properties with a 0
  and a -4002 code, and an action that ran.
- python-miio's `Gateway`: `discover_devices` found both sub-devices
  with their zigbee models and firmware from device_list; the weather
  sensor's `update` read 23.15 and 48.2 through get_device_prop_exp;
  `get_battery`; `set_property`.
- python-miio's firmware update (`update`, `update_state`,
  `update_progress`): the simulator downloaded the image from a test
  server, checked the MD5 and went downloading, installing, idle at 100.
- python-miio's `RoborockVacuum` (start, status with state code and
  battery, home) and `ChuangmiPlug` for chuangmi.plug.v1 (on, status).
- python-miio's `PushServer` subscribed to the door sensor's "open" on
  the simulated gateway with send_data_frame; `Emit` pushed the event
  to it, python-miio answered and its handler got the source and
  parameters; `unsubscribe_event` removed the scene with miIO.xdel.
  Docker translates the host's address, so the test peer also accepts
  pushes from the container's default gateway.
- micloud 0.6 (python-miio's Mi account client), its fixed Xiaomi
  addresses pointed at the simulator: sign-in (form-encoded, where our
  client sends query parameters; the simulator now reads both), the
  older home/device_list with the token, and miotspec/prop/get through
  a linked device. The run made the simulator take form bodies and
  answer "result": "ok" at sign-in, which micloud requires.
- Not yet [hardware]: compared with a real Xiaomi device, gateway or
  sub-device, a real firmware image, or a real device's push.
- Not yet [vendor account]: compared with a real Mi account, its captcha
  and identity check, or its device-control paths. The simulators and
  the miio client are written by the same author, so their agreement is
  not independent; the python-miio and micloud runs are.

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
