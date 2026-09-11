#!/usr/bin/env python3
"""End-to-end TUI check for gotopr without a real terminal.

Spawns the binary on a pty, answers the terminal queries bubbletea sends
(OSC 10/11, CSI 6n, DA1), replays keystrokes and SGR mouse wheel bursts, and
asserts on frames rendered with pyte. Everything runs in a throwaway sandbox
(fake clones under GOTOPR_ROOT, a synthetic prcache.json, a fake herdr), so it
never touches the real plugin state.

Usage: scripts/pty-check.py ./gotopr [dark|light]   (needs python3 + pyte)
"""
import datetime, fcntl, json, os, pty, select, shutil, struct, subprocess, sys, tempfile, termios, time
import pyte

BIN = os.path.abspath(sys.argv[1])
BG = sys.argv[2] if len(sys.argv) > 2 else "dark"
ROWS, COLS = 10, 100
SANDBOX = tempfile.mkdtemp(prefix="gotopr-pty-")

# ---------- sandbox: fake clones, synthetic cache, fake herdr ----------
root = os.path.join(SANDBOX, "root"); state = os.path.join(SANDBOX, "state"); bind = os.path.join(SANDBOX, "bin")
for d in (root, state, bind): os.makedirs(d)
repos = ["alpha", "beta", "gamma"]
for r in repos:
    os.makedirs(os.path.join(root, r, ".git"))
    with open(os.path.join(root, r, ".git", "config"), "w") as f:
        f.write('[remote "origin"]\n\turl = git@github.com:acme/%s.git\n' % r)
now = datetime.datetime.now(datetime.timezone.utc)
prs = []
for i in range(15):
    r = repos[i % 3]
    body = "\n".join("Line %02d of the description for PR %d, with some **markdown** text." % (n, 100 + i) for n in range(1, 41))
    prs.append({
        "url": "https://github.com/acme/%s/pull/%d" % (r, 100 + i), "number": 100 + i,
        "title": "PR %d title for %s" % (100 + i, r), "body": body,
        "updated_at": (now - datetime.timedelta(minutes=i)).isoformat().replace("+00:00", "Z"),
        "head_ref_name": "feat/pr-%d" % (100 + i), "is_draft": False, "is_cross_repo": False,
        "head_repo_owner": "acme", "repo_slug": "acme/" + r, "roles": ["author"],
    })
with open(os.path.join(state, "prcache.json"), "w") as f:
    json.dump({"fetched_at": now.isoformat().replace("+00:00", "Z"), "prs": prs}, f)
herdr_log = os.path.join(SANDBOX, "herdr.log")
fake = os.path.join(bind, "herdr")
with open(fake, "w") as f:
    f.write('#!/bin/sh\necho "$@" >> "%s"\n' % herdr_log)
os.chmod(fake, 0o755)

# ---------- spawn ----------
env = dict(os.environ, TERM="xterm-256color", COLORTERM="truecolor", GOTOPR_ROOT=root,
           HERDR_PLUGIN_STATE_DIR=state, HERDR_BIN_PATH=fake)
env.pop("HERDR_ENV", None)
master, slave = pty.openpty()
fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", ROWS, COLS, 0, 0))
proc = subprocess.Popen([BIN], stdin=slave, stdout=slave, stderr=slave, env=env, close_fds=True, cwd=SANDBOX)
os.close(slave)

screen = pyte.Screen(COLS, ROWS)
stream = pyte.ByteStream(screen)
raw = bytearray()
answered = 0

BGREPLY = b"\x1b]11;rgb:0000/0000/0000\x1b\\" if BG == "dark" else b"\x1b]11;rgb:ffff/ffff/ffff\x1b\\"
FGREPLY = b"\x1b]10;rgb:ffff/ffff/ffff\x1b\\" if BG == "dark" else b"\x1b]10;rgb:0000/0000/0000\x1b\\"
QUERIES = [(b"\x1b]11;?", BGREPLY), (b"\x1b]10;?", FGREPLY), (b"\x1b[6n", b"\x1b[1;1R"), (b"\x1b[c", b"\x1b[?62c")]

def pump(seconds):
    """Read program output for `seconds`, feeding pyte and answering queries."""
    global answered
    end = time.time() + seconds
    while True:
        left = end - time.time()
        if left <= 0: break
        r, _, _ = select.select([master], [], [], left)
        if not r: continue
        try:
            data = os.read(master, 65536)
        except OSError:
            break
        if not data: break
        raw.extend(data); stream.feed(data)
        # answer every query that appeared since the last scan
        tail = bytes(raw[answered:])
        for q, reply in QUERIES:
            for _ in range(tail.count(q)):
                os.write(master, reply)
        answered = len(raw)

def frame():
    return [line.rstrip() for line in screen.display]

def send(b):
    os.write(master, b)

failures = []
def check(cond, msg):
    print(("  ok   " if cond else "  FAIL ") + msg)
    if not cond: failures.append(msg)

def listw():
    return max((COLS - 3) * 30 // 100, 20)

def left(f):  return [l[:listw()] for l in f[1:-1]]
def right(f): return [l[listw() + 3:] for l in f[1:-1]]

def dump(title, f):
    print("--- %s ---" % title)
    for i, l in enumerate(f): print("%2d|%s" % (i, l))

# ---------- steps ----------
print("== gotopr pty driver (%s background, %dx%d) ==" % (BG, COLS, ROWS))
for _ in range(50):
    pump(0.1)
    if "gotopr (dev) ❯" in frame()[0]: break
pump(0.6)
f0 = frame(); dump("initial frame", f0)
prompt = f0[0]
check(prompt.startswith("gotopr (dev) ❯") and prompt.strip() == "gotopr (dev) ❯", "prompt line is clean: %r" % prompt)
check("▌" in f0[1] or any("▌" in l for l in left(f0)), "selection marker visible in list")
check(b"\x1b[?1002h" in raw and b"\x1b[?1006h" in raw, "program requested cell-motion + SGR mouse modes")
check(b"\x1b[?1049h" in raw, "program entered the alt screen")

# 1. burst over the preview (60 wheel-down reports in one write)
send(b"\x1b[<65;70;8M" * 60); pump(0.5)
f1 = frame(); dump("after 60x wheel-down over preview", f1)
check(f1[0].strip() == "gotopr (dev) ❯", "prompt still clean after preview burst: %r" % f1[0])
check(left(f1) == left(f0), "list column unchanged by preview wheel")
check(right(f1)[:2] == right(f0)[:2], "preview header unchanged")
check(right(f1)[3:] != right(f0)[3:], "preview body shifted")

# 2. wheel over the list also scrolls the preview and never moves the selection
send(b"\x1b[<64;70;8M" * 60); pump(0.5)  # wheel-up over the preview: back to the top
f1b = frame()
check(right(f1b) == right(f0), "wheel-up over the preview returns it to the top")
send(b"\x1b[<65;5;8M"); pump(0.5)
f2 = frame(); dump("after 1x wheel-down over list", f2)
check(f2[0].strip() == "gotopr (dev) \u276f", "prompt still clean after list wheel: %r" % f2[0])
check(left(f2) == left(f0), "wheel over the list leaves the selection alone")
check(right(f2)[3:] != right(f0)[3:], "wheel over the list scrolls the preview body")

# 3. a burst over the list keeps the selection and keeps scrolling the preview
send(b"\x1b[<65;5;8M" * 60); pump(0.5)
f3 = frame(); dump("after 60x wheel-down over list", f3)
check(f3[0].strip() == "gotopr (dev) \u276f", "prompt still clean after list burst: %r" % f3[0])
check(left(f3) == left(f0), "list burst leaves the selection alone")
send(b"\x1b[<64;5;8M" * 60); pump(0.5)
f3b = frame()
check(right(f3b) == right(f0), "wheel-up over the list scrolls the preview back to the top")

# 4. arrow down still moves the selection (key map unchanged)
send(b"\x1b[B"); pump(0.5)
f4 = frame(); dump("after down arrow", f4)
check(any("\u258c #103" in l for l in left(f4)), "down arrow moves to the second PR (#103)")
check("#103" in right(f4)[1], "preview header shows #103")

# 4b. left click on a list row selects it without opening; header/preview clicks are inert
send(b"\x1b[<0;5;5M\x1b[<0;5;5m"); pump(0.5)   # press+release on screen line 5 (row 4 of the list)
f4b = frame(); dump("after click on list row 4", f4b)
clicked = [l for l in left(f4b) if "\u258c" in l]
check(len(clicked) == 1 and "#103" not in clicked[0], "click moved the selection off #103: %r" % (clicked[:1],))
check(clicked and clicked[0].split()[1] in right(f4b)[1], "preview header follows the clicked PR")
send(b"\x1b[<0;5;2M\x1b[<0;5;2m"); pump(0.4)   # screen line 2 is the first group header
f4c = frame()
check(left(f4c) == left(f4b), "click on a header changes nothing")
send(b"\x1b[<0;70;5M\x1b[<0;70;5m"); pump(0.4)
f4d = frame()
check(left(f4d) == left(f4b), "click on the preview changes nothing")

# 5. typing still filters; backspace clears
send(b"gamma"); pump(0.5)
f5 = frame(); dump("after typing 'gamma'", f5)
check(f5[0].strip() == "gotopr (dev) ❯ gamma", "typed text lands in the filter: %r" % f5[0])
check(not any("alpha" == l.strip() for l in left(f5)), "filter narrowed the list (no alpha header)")
send(b"\x7f" * 5); pump(0.4)
f6 = frame()
check(f6[0].strip() == "gotopr (dev) ❯", "backspace clears the filter: %r" % f6[0])

# 6. shift+down scrolls the preview from the keyboard (key map unchanged)
send(b"\x1b[1;2B"); pump(0.4)
f7 = frame()
check(right(f7)[3:] != right(f6)[3:], "shift+down scrolls the preview")

# 7. garbage scan over every frame captured so far
allframes = "\n".join("\n".join(f) for f in (f0, f1, f2, f3, f3b, f4, f4b, f4c, f4d, f5, f6, f7))
check("<6" not in allframes and "rgb:" not in allframes and "[<" not in allframes, "no mouse/OSC garbage in any frame")

# 8. esc quits cleanly, mouse modes reset, no herdr action
send(b"\x1b"); pump(1.0)
try:
    code = proc.wait(timeout=3)
except subprocess.TimeoutExpired:
    proc.kill(); code = "timeout"
check(code == 0, "exit code 0 on esc (got %r)" % code)
check(b"\x1b[?1002l" in raw or b"\x1b[?1006l" in raw, "mouse modes reset on exit")
check(not os.path.exists(herdr_log), "no herdr action after esc")

print("== %d failure(s) ==" % len(failures))
shutil.rmtree(SANDBOX, ignore_errors=True)
sys.exit(1 if failures else 0)
