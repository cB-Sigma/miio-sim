# Changelog

All notable changes to this module. Versions follow semantic versioning;
before v1 the API may change in minor versions.

## v0.3.0 (2026-10-12)

Additions only; v0.2.0 code keeps working. Needs miio v0.3.0.

- Roborock vacuums answer the legacy extras: get_consumable,
  reset_consumable, get_clean_summary and get_clean_record (in the older
  list shape, or the newer object shape with
  `VacuumState.ObjectAnswers`), get_dnd_timer, set_dnd_timer,
  close_dnd_timer, get_timer, set_timer, upd_timer, del_timer,
  get_timezone, get_sound_volume, change_sound_volume,
  test_sound_volume, get_custom_mode, set_custom_mode, get_carpet_mode,
  set_carpet_mode, find_me and app_spot; their state is
  `Device.Vacuum` (`VacuumState`, `NewVacuumState`, `VacuumRecord`,
  `VacuumTimer`).
- Every device answers miIO.config_router and keeps what it asked for
  in `Device.WiFi`.
- mDNS: `Device.MDNSInstance` and `Browser`, a miio.Browser listing
  simulated devices (nothing is sent on the network).
- The specification service serves translations
  (/instance/v2/multiLanguage, `SetTranslations`), answers status=all
  with each entry's status, and can be made to fail (`SetDown`) to test
  a mirror offline.
- `miiosim -vacuum` (a roborock.vacuum.s5) and `-specs <dir>` (serves a
  specification mirror directory).
- Interop: python-miio's RoborockVacuum extras, configure_wifi and
  supports_miot against the simulated vacuum.

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
