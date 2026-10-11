# Changelog

All notable changes to this module. Versions follow semantic versioning;
before v1 the API may change in minor versions.

## v0.2.0 (2026-10-11)

Additions only; v0.1.0 code keeps working. Needs miio v0.2.0.

- Gateways: lumi gateways answer the sub-device methods for their
  `SubDevices` (device_list, get_device_list on lumi.gateway.mgl03,
  get_device_prop, get_device_prop_exp, set_device_prop, get_battery,
  remove_device); `SubProp`, `DemoSubDevices`, `GatewayToken`.
- Roborock vacuums: app_start, app_stop, app_pause, app_charge,
  get_status.
- Firmware update: miIO.ota downloads the image, checks its MD5 and
  installs `OTAVersion`; miIO.get_ota_progress, miIO.get_ota_state;
  `FirmwareVersion`.
- Pushes: send_data_frame scenes, miIO.xdel, `Scenes`, `Emit` (with
  `PushTo` for a test listener).
- Mi account: `Captcha` and an identity check by e-mail or SMS that can
  now be completed (`Verify`, `Code`, `CaptchaText`, `CodesSent`,
  `Verified`); device control for `Link`ed devices; the older
  home/device_list; parameters read from a form body as well as the
  query, and "result": "ok" at sign-in (found by running micloud).
- `miiosim -gateway` and `-verify`.
- Interop: runs on python:3.12.10-slim; python-miio's Gateway, firmware
  update, RoborockVacuum, ChuangmiPlug and PushServer, and micloud 0.6,
  were added as peers.

## v0.1.0 (2026-10-11)

First release as a module of its own, split from the miio module:
simulated miIO and MIoT devices (`Start`, `Device`, `NewDemo`), the Mi
account (`StartCloud`), the specification service (`StartSpecServer`)
and `cmd/miiosim`.

- `StartAt` serves a device on a given UDP address, such as the miIO
  port 54321 that python-miio's hello goes to.
- The miio client's tests against the simulators moved here from miio.
- Interop tests running python-miio's client against the simulators, in
  a nested test-only module, `interop`.
