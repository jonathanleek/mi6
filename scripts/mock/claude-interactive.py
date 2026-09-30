"""Drive an interactive Claude Code session through a pty: two prompts, then exit.

usage: python3 interactive.py <claude binary> <cwd>
Writes the raw screen output to $MOCK_SCREEN, or discards it.
"""
import os, pty, select, sys, time

binary, cwd = sys.argv[1], sys.argv[2]
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
            # Answer a terminal size or cursor position query so the TUI can draw.
            if b"\x1b[6n" in data:
                os.write(fd, b"\x1b[1;1R")


def send(text):
    for ch in text:
        os.write(fd, ch.encode())
        time.sleep(0.02)


pump(10)
send("Say hi.")
os.write(fd, b"\r")
pump(15)
send("Say hi again.")
os.write(fd, b"\r")
pump(15)
send("/exit")
os.write(fd, b"\r")
pump(5)
try:
    os.kill(pid, 9)
except ProcessLookupError:
    pass
