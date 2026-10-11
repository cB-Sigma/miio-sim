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

v0.2.0 (2026-10-11): sim/gateway.go (gateway sub-devices and the legacy
Roborock methods), sim/ota.go, sim/push.go, sim/verify.go and
sim/cloudrpc.go were written for this module from the facts the miio
module's v0.2.0 uses (its PROVENANCE.md lists them; python-miio at
9a00e08 read for behaviour only, token_extractor.py for the
verification steps). The demo sub-devices' sids are made up; their
zigbee models and type numbers are real ones so clients map them. The
interop runs moved to python:3.12.10-slim and were repeated, and
python-miio's Gateway, firmware update, RoborockVacuum, ChuangmiPlug and
PushServer and micloud 0.6 were added as peers. The micloud run changed
the simulated account: it now reads parameters from a form body as well
as the query, answers "result": "ok" at sign-in, and serves the older
home/device_list.

v0.3.0 (2026-10-12): sim/vacuum.go and sim/mdns.go were written for
this module, and the specification service and the command were
extended, from the facts the miio module's v0.3.0 uses (its
PROVENANCE.md lists them: python-miio at 9a00e08 read for the Roborock
method names and answer fields, hass-xiaomi-miot v1.1.5 for the
translations path). The demo consumable counters, cleaning records,
timer, volume, fan speed, carpet thresholds and time zone are our own
made-up values, not python-miio's simulator data. python-miio's
RoborockVacuum then ran against the simulated vacuum with no
simulator change needed.

## Sources

| Area (files) | Followed |
|---|---|
| Devices (sim/sim.go, sim/miot.go) | the miio module's packets; method sets and error answers of the legacy models from python-miio's descriptions (facts only); MIoT codes -4002, -4003, -4005 |
| Mi account and specification service (sim/cloud.go) | Xiaomi-cloud-tokens-extractor's steps (MIT, facts only); miot-spec.org's format |
| Gateways, vacuums, firmware update, pushes (sim/gateway.go, sim/ota.go, sim/push.go) | python-miio's descriptions of the gateway sub-device methods, the Roborock methods and state codes, miIO.ota and its states, the push scene (facts only); the timing, the real download and MD5 check, and `Emit` are our own design |
| Account verification and device control (sim/verify.go, sim/cloudrpc.go) | token_extractor.py's identity-check and captcha steps (MIT, facts only); the device-control paths as open-source Mi account clients use them; the context check, demo code and captcha image are our own |
| Vacuum extras, Wi-Fi setup, mDNS names (sim/vacuum.go, sim/sim.go, sim/mdns.go) | python-miio's descriptions of the Roborock legacy methods and answer shapes, miIO.config_router and the mDNS instance name (facts only); the checks and refusals are our own design |
| Translations (sim/cloud.go) | hass-xiaomi-miot's description of the multiLanguage path and its "data" member (Apache-2.0, facts only) |
| Command (cmd/miiosim) | none (flags are our own) |
| Interop peer (interop/peer/client.py) | our own script calling python-miio at a fixed commit and micloud, installed at test time |

No specification or vendor text is reproduced.

## Tests

| Test | Source |
|---|---|
| client_test.go, example_test.go, sim/, cmd/miiosim | the miio client against these simulators, written from the same descriptions; not a real device, account or specification service |
| interop/ | python-miio's client and micloud against these simulators |

All tokens, device IDs, accounts and addresses are made-up demo values.
