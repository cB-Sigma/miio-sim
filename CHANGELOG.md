# Changelog

All notable changes to this module. Versions follow semantic versioning;
before v1 the API may change in minor versions.

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
