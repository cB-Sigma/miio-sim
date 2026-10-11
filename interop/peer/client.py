# Copyright 2026 Abhinav Gautam
# SPDX-License-Identifier: Apache-2.0
#
# python-miio's client against a simulated device, for interop tests.
#
#   python client.py legacy <host> <port> <token hex>
#   python client.py miot <host> <port> <token hex> <did>
#   python client.py wrongtoken <host> <port>
#
# Prints "PEER <step> <json>" after each step and "PEER done" at the end.

import json
import sys

import miio
from miio.miioprotocol import MiIOProtocol


def say(step, obj=None):
    print("PEER " + step + ("" if obj is None else " " + json.dumps(obj, sort_keys=True, default=str)), flush=True)


def device(host, port, token):
    d = miio.Device(host, token, timeout=3)
    d._protocol.port = int(port)
    return d


def legacy(host, port, token):
    p = MiIOProtocol(host, token, timeout=3)
    p.port = int(port)
    m = p.send_handshake()
    say("hello", {"device_id": int.from_bytes(m.header.value.device_id, "big"), "ts": str(m.header.value.ts)})
    d = device(host, port, token)
    info = d.info()
    say("info", {"model": info.model, "firmware": info.firmware_version, "mac": info.mac_address})
    say("get_prop", d.send("get_prop", ["power", "temperature"]))
    say("set_power", d.send("set_power", ["off"]))
    say("get_prop2", d.send("get_prop", ["power"]))
    say("done")


def miot(host, port, token, did):
    d = device(host, port, token)
    say("get_properties", d.send("get_properties", [{"did": did, "siid": 2, "piid": 2}, {"did": did, "siid": 3, "piid": 6}, {"did": did, "siid": 9, "piid": 9}]))
    say("set_properties", d.send("set_properties", [{"did": did, "siid": 2, "piid": 2, "value": False}, {"did": did, "siid": 3, "piid": 6, "value": 1}]))
    say("action", d.send("action", {"did": did, "siid": 4, "aiid": 1, "in": []}))
    say("done")


def wrongtoken(host, port):
    d = device(host, port, "0123456789abcdef0123456789abcdef")
    try:
        r = d.send("miIO.info", [])
        say("answered", r)
    except Exception as e:
        say("refused", {"error": type(e).__name__})
    say("done")


kind = sys.argv[1]
if kind == "legacy":
    legacy(*sys.argv[2:5])
elif kind == "miot":
    miot(*sys.argv[2:6])
else:
    wrongtoken(*sys.argv[2:4])
