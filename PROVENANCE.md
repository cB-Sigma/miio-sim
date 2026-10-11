# Provenance

## How the code was written

Written for SigmaM2M, a oneM2M platform, in Go, by the author of the miio
module and from the same public descriptions (facts only; python-miio is
GPL-3.0 and no code was taken from it). The legacy device simulator came
with SigmaM2M's first commit (2026-09-27); the Philips, chuangmi v1/v3
and Yeelight method sets were added on 2026-10-05 after reading
python-miio's philips_bulb.py and chuangmi_plug.py and the Yeelight
Inter-Operation Specification; the MIoT answers, the Mi account and the
specification service on 2026-10-05. A data race in `NewDemo` (flags set
after the device started serving) was fixed on 2026-10-06. The
simulators, their command and the tests that drive them moved out of
the miio module into this one on 2026-10-11 (with `git mv`).

On 2026-10-11 python-miio's client was run against the simulators
(`interop`). python-miio sends its hello only to port 54321, so
`StartAt` was added to serve a device on a given address; no simulator
behaviour had to change.

## Sources

| Area (files) | Followed |
|---|---|
| Devices (sim/sim.go, sim/miot.go) | the miio module's packets; method sets and error answers of the legacy models from python-miio's descriptions (facts only); MIoT codes -4002, -4003, -4005 |
| Mi account and specification service (sim/cloud.go) | Xiaomi-cloud-tokens-extractor's steps (MIT, facts only); miot-spec.org's format |
| Command (cmd/miiosim) | none (flags are our own) |
| Interop peer (interop/peer/client.py) | our own script calling python-miio at a fixed commit, installed at test time |

No specification or vendor text is reproduced.

## Tests

| Test | Source |
|---|---|
| client_test.go, example_test.go, sim/, cmd/miiosim | the miio client against these simulators, written from the same descriptions; not a real device, account or specification service |
| interop/ | python-miio's client against these simulators |

All tokens, device IDs, accounts and addresses are made-up demo values.
