"""Drive an interactive Claude Code session through a pty: type each /model
switch in MODEL_SWITCHES (comma-separated), send a prompt after each, exit.

usage: python3 claude-model-switch.py <claude binary> <cwd>
The requests the fake server logs say whether any switch took.
"""
import os, pty, select, sys, time

binary, cwd = sys.argv[1], sys.argv[2]
switches = [s for s in os.environ.get("MODEL_SWITCHES", "/model opus").split(",") if s]
out = open(os.environ.get("MOCK_SCREEN", os.devnull), "wb")

pid, fd = pty.fork()
if pid == 0:
    os.chdir(cwd)
    os.execv(binary, [binary])


def pump(seconds):
    end = time.time() + seconds
    while time.time() < end:
        r, _, _ = select.select([fd], [], [], 0.2)
        if r:
            try:
                data = os.read(fd, 65536)
            except OSError:
                return
            out.write(data)
            if b"\x1b[6n" in data:
                os.write(fd, b"\x1b[1;1R")


def send(text):
    for ch in text:
        os.write(fd, ch.encode())
        time.sleep(0.03)


pump(8)
for cmd in switches:
    send(cmd)
    pump(2)
    os.write(fd, b"\r")
    pump(3)
    send("Say hi.")
    os.write(fd, b"\r")
    pump(8)
send("/exit")
os.write(fd, b"\r")
pump(3)
try:
    os.kill(pid, 9)
except ProcessLookupError:
    pass
