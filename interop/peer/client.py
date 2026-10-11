# Copyright 2026 Abhinav Gautam
# SPDX-License-Identifier: Apache-2.0
#
# python-miio's client against a simulated device, for interop tests.
#
#   python client.py legacy <host> <port> <token hex>
#   python client.py miot <host> <port> <token hex> <did>
#   python client.py wrongtoken <host> <port>
#   python client.py gateway <host> <port> <token hex> <sub-device sid>
#   python client.py ota <host> <port> <token hex> <url> <md5>
#   python client.py vacuum|plug <host> <port> <token hex>
#   python client.py push <host> <port> <token hex> <sub-device sid>
#     (subscribes, then waits for a line on stdin before unsubscribing)
#   python client.py micloud <account base URL> <user> <password> <did>
#     (micloud, python-miio's Mi account client, against the simulator)
#   -v anywhere logs python-miio's debug output.
#
# Prints "PEER <step> <json>" after each step and "PEER done" at the end.

import asyncio
import json
import logging
import socket
import struct
import sys
import time

import miio
from miio import ChuangmiPlug, EventInfo, Gateway, PushServer, RoborockVacuum
from miio.miioprotocol import MiIOProtocol

logging.basicConfig(level=logging.DEBUG if "-v" in sys.argv else logging.WARNING)
sys.argv = [a for a in sys.argv if a != "-v"]


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


def gateway(host, port, token, sid):
    gw = Gateway(host, token, model="lumi.gateway.v3", timeout=3)
    gw._protocol.port = int(port)
    devs = gw.discover_devices()
    say("devices", {s: {"zigbee": d.zigbee_model, "fw": d.firmware_version} for s, d in devs.items()})
    ht = devs[sid]
    ht.update()
    say("ht", ht.status)
    say("battery", ht.get_battery())
    say("set", ht.set_property("alias", "kitchen"))
    say("done")


def ota(host, port, token, url, md5):
    d = device(host, port, token)
    say("update", d.update(url, md5))
    states = []
    for _ in range(100):
        st = d.update_state().value
        if not states or states[-1] != st:
            states.append(st)
        if st in ("idle", "failed") and len(states) > 1:
            break
        time.sleep(0.05)
    say("states", {"states": states, "progress": d.update_progress()})
    say("done")


def vacuum(host, port, token):
    v = RoborockVacuum(host, token, model="roborock.vacuum.s5", timeout=3)
    v._protocol.port = int(port)
    say("start", v.start())
    st = v.status()
    say("status", {"state": st.state_code, "battery": st.battery})
    say("home", v.home())
    say("done")


def plug(host, port, token):
    p = ChuangmiPlug(host, token, model="chuangmi.plug.v1", timeout=3)
    p._protocol.port = int(port)
    say("on", p.on())
    st = p.status()
    say("status", {"on": st.is_on, "temperature": st.temperature})
    say("done")


def route_gateway():
    """The container's default gateway, where host traffic appears from."""
    with open("/proc/net/route") as f:
        for line in f.readlines()[1:]:
            parts = line.split()
            if parts[1] == "00000000":
                return socket.inet_ntoa(struct.pack("<L", int(parts[2], 16)))
    return None


async def push(host, port, token, sid):
    ip = socket.gethostbyname(host)
    server = PushServer(device_ip=ip)
    await server.start()
    gw = Gateway(ip, token, model="lumi.gateway.v3", timeout=3)
    gw._protocol.port = int(port)

    def callback(source, action, params):
        say("event", {"source": source, "action": action, "params": params})

    server.register_miio_device(gw, callback)
    # Pushes from the host reach the container through Docker's address
    # translation: accept them from the default gateway as well.
    alt = route_gateway()
    if alt and alt != ip:
        server._registered_devices[alt] = server._registered_devices[ip]
    info = EventInfo(action="open", extra="[1,6,1,0,[0,1],2,0]", source_sid=sid, source_model="lumi.sensor_magnet.v2")
    eid = await server.subscribe_event(gw, info)
    say("subscribed", {"id": eid, "server_ip": server.server_ip, "also": alt})
    loop = asyncio.get_running_loop()
    await loop.run_in_executor(None, sys.stdin.readline)
    res = await server.unsubscribe_event(gw, eid)
    say("unsubscribed", res)
    say("done")


def micloud_run(base, user, password, did):
    """micloud (python-miio's Mi account client) against the simulated
    account: its fixed Xiaomi addresses are pointed at the simulator."""
    import re

    import micloud.micloud as mm

    orig = mm.get_session

    def rewrite(url):
        url = url.replace("https://account.xiaomi.com", base)
        url = re.sub(r"https://(\w+)\.api\.io\.mi\.com", lambda m: base + "/" + m.group(1), url)
        return url.replace("http://127.0.0.1:", "http://host.docker.internal:")

    def session():
        s = orig()
        real = s.request
        s.request = lambda method, url, *a, **k: real(method, rewrite(url), *a, **k)
        return s

    mm.get_session = session
    cloud = mm.MiCloud(user, password)
    say("login", cloud.login())
    devs = cloud.get_devices(country="i2") or []
    say("devices", [{"did": d.get("did"), "token": d.get("token"), "model": d.get("model")} for d in devs])
    r = cloud.request_country("/miotspec/prop/get", "i2", {"data": json.dumps({"params": [{"did": did, "siid": 3, "piid": 6}]})})
    say("prop", json.loads(r))
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
elif kind == "gateway":
    gateway(*sys.argv[2:6])
elif kind == "ota":
    ota(*sys.argv[2:7])
elif kind == "vacuum":
    vacuum(*sys.argv[2:5])
elif kind == "plug":
    plug(*sys.argv[2:5])
elif kind == "push":
    asyncio.run(push(*sys.argv[2:6]))
elif kind == "micloud":
    micloud_run(*sys.argv[2:6])
else:
    wrongtoken(*sys.argv[2:4])
