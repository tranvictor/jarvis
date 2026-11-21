"""Press the confirm (or reject) button on a Trezor emulator whenever it asks.

Usage: autoconfirm.py [yes|no] [host:port]

Talks to the emulator's debug link, so it needs a firmware built with
`xtask build firmware --emulator -m t2t1 -p test`.
"""
import sys
import time

from trezorlib import models
from trezorlib.debuglink import DebugLink
from trezorlib.transport.udp import UdpTransport

mode = sys.argv[1] if len(sys.argv) > 1 else "yes"
addr = sys.argv[2] if len(sys.argv) > 2 else "127.0.0.1:21324"

debug = DebugLink(UdpTransport(addr).find_debug())
debug.model = models.T2T1
debug.version = (2, 9, 0)

IDLE = {"Homescreen", "Lockscreen", "Busyscreen"}
last = None
repeats = 0
while True:
    try:
        layout = debug.read_layout(wait=False)
        name = layout.main_component()
        text = layout.text_content()
    except Exception as exc:  # emulator restarting
        print("debuglink:", exc, flush=True)
        time.sleep(0.5)
        continue
    if name in IDLE:
        last, repeats = None, 0
        time.sleep(0.15)
        continue
    if (name, text) != last:
        print(f"layout {name}: {text} -> {mode}", flush=True)
        last, repeats = (name, text), 0
    else:
        repeats += 1
    buttons = debug.screen_buttons
    target = buttons.ok() if mode == "yes" else buttons.cancel()
    # A plain tap advances most screens; "hold to confirm" screens need a hold,
    # so fall back to one when the same screen stays up.
    hold = 1200 if (mode == "yes" and repeats >= 2) else None
    try:
        debug.click(target, hold_ms=hold, wait=True)
    except Exception as exc:
        print("click:", exc, flush=True)
    time.sleep(0.3)
