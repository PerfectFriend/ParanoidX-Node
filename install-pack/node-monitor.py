#!/usr/bin/python3
"""node-monitor — Wayland-aware system tray monitor for simplex-node (sequential poller, low resources)."""
import os, sys, fcntl

# ── Single-instance lock ─────────────────────────────────────────────────────
PID_FILE = os.path.join(os.environ.get("DATA_DIR", os.path.expanduser("~/.local/share/simplex-node")), "node-monitor.pid")
os.makedirs(os.path.dirname(PID_FILE), exist_ok=True)

def _acquire_lock():
    """Acquire PID file lock — exit if another instance is running."""
    lf = open(PID_FILE, "w")
    try:
        fcntl.flock(lf, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except IOError:
        print(f"node-monitor: another instance is running (PID file: {PID_FILE})", file=sys.stderr)
        sys.exit(1)
    lf.write(str(os.getpid()) + "\n")
    lf.flush()
    return lf

_LOCK_FD = _acquire_lock()
# ──────────────────────────────────────────────────────────────────────────────

# point GI at our local typelib (AyatanaAppIndicator3)
_typ = os.environ.get("GI_TYPELIB_PATH", "")
_tl = os.path.expanduser("~/.local/share/girepository-1.0")
if os.path.isdir(_tl):
    os.environ["GI_TYPELIB_PATH"] = f"{_tl}:{_typ}" if _typ else _tl

import gi
gi.require_version('Gtk', '3.0')
gi.require_version('AyatanaAppIndicator3', '0.1')
from gi.repository import Gtk, GLib, GdkPixbuf, AyatanaAppIndicator3
try:
    gi.require_version('Notify', '0.7')
    from gi.repository import Notify
    Notify.init("node-monitor")
    _HAVE_NOTIFY = True
except (ImportError, ValueError):
    _HAVE_NOTIFY = False
    log("Notify not available, falling back to notify-send")
import subprocess, time, threading, json, urllib.request, urllib.error, socket, atexit
from PIL import Image, ImageDraw, ImageFont
from io import BytesIO

API = os.environ.get("NODE_API", "http://127.0.0.1:8080")
NODE_BIN = os.environ.get("NODE_BIN", os.path.expanduser("~/bin/simplex-node"))
POLL_INTERVAL = int(os.environ.get("POLL_INTERVAL", "120"))
DATA_DIR = os.environ.get("DATA_DIR", os.path.expanduser("~/.local/share/simplex-node"))
ICON_DIR = os.path.join(DATA_DIR, "icons")
os.makedirs(ICON_DIR, exist_ok=True)
LOG = os.path.join(DATA_DIR, "node-monitor.log")

def log(msg):
    ts = time.strftime("%Y-%m-%d %H:%M:%S")
    with open(LOG, "a") as f:
        f.write(f"{ts} {msg}\n")


def fetch_json(path, timeout=10):
    url = API.rstrip("/") + path
    req = urllib.request.Request(url, headers={"Accept": "application/json"})
    try:
        resp = urllib.request.urlopen(req, timeout=timeout)
        return json.loads(resp.read().decode())
    except Exception as e:
        return {"_error": str(e)}


def system_info():
    import psutil
    cpu = psutil.cpu_percent(interval=0.5)
    mem = psutil.virtual_memory()
    disk = psutil.disk_usage("/")
    boot = psutil.boot_time()
    net = psutil.net_if_addrs()
    ips = []
    for iface, addrs in net.items():
        for a in addrs:
            if a.family == socket.AF_INET and a.address != "127.0.0.1":
                ips.append(f"{iface}: {a.address}")
    return {
        "cpu": cpu,
        "mem_used": mem.used, "mem_total": mem.total, "mem_percent": mem.percent,
        "disk_used": disk.used, "disk_total": disk.total, "disk_percent": disk.percent,
        "uptime": int(time.time() - boot),
        "hostname": socket.gethostname(),
        "ips": ips or ["127.0.0.1"],
    }


def fmt_bytes(b):
    for u in ("B", "KiB", "MiB", "GiB", "TiB"):
        if b < 1024:
            return f"{b:.1f} {u}"
        b /= 1024
    return f"{b:.1f} PiB"


def fmt_uptime(s):
    d, s = divmod(int(s), 86400)
    h, s = divmod(s, 3600)
    m, s = divmod(s, 60)
    parts = []
    if d: parts.append(f"{d}d")
    if h: parts.append(f"{h}h")
    parts.append(f"{m}m")
    return " ".join(parts)


ICON_SIZE = 48

def _make_icon_pil(status):
    colors = {
        "healthy": ((0, 180, 80), (0, 220, 100)),
        "degraded": ((210, 160, 0), (255, 210, 30)),
        "down": ((200, 40, 40), (250, 70, 70)),
        "syncing": ((30, 100, 220), (60, 140, 255)),
        "unknown": ((120, 120, 120), (170, 170, 170)),
    }
    c1, c2 = colors.get(status, colors["unknown"])
    im = Image.new("RGBA", (ICON_SIZE, ICON_SIZE), (0, 0, 0, 0))
    draw = ImageDraw.Draw(im)
    cx = cy = ICON_SIZE // 2
    r = ICON_SIZE // 2 - 1

    # outer bevel
    for i in range(r, r - 6, -1):
        t = (r - i) / 6.0
        cr = tuple(int(a + (b - a) * t) for a, b in zip(c1, c2))
        draw.ellipse((cx - i, cy - i, cx + i, cy + i), fill=cr + (255,))

    # inner shadow ring
    draw.ellipse((cx - r + 6, cy - r + 6, cx + r - 6, cy + r - 6), fill=(0, 0, 0, 40))
    draw.ellipse((cx - r + 8, cy - r + 8, cx + r - 8, cy + r - 8), fill=c2 + (255,))

    # glossy highlight (top half crescent)
    grad = Image.new("RGBA", (ICON_SIZE, ICON_SIZE), (0, 0, 0, 0))
    gdraw = ImageDraw.Draw(grad)
    gdraw.ellipse((cx - r + 8, cy - r + 8, cx + r - 8, cy + r - 8), fill=(255, 255, 255, 40))
    # clip bottom half
    mask = Image.new("L", (ICON_SIZE, ICON_SIZE), 0)
    mdraw = ImageDraw.Draw(mask)
    mdraw.rectangle((0, cy + 2, ICON_SIZE, ICON_SIZE), fill=255)
    grad_masked = Image.new("RGBA", (ICON_SIZE, ICON_SIZE), (0, 0, 0, 0))
    grad_masked.paste(grad, (0, 0), mask)
    im = Image.alpha_composite(im, grad_masked)

    # subtle inner glow dot at center
    draw.ellipse((cx - 3, cy - 3, cx + 3, cy + 3), fill=(255, 255, 255, 30))
    draw.ellipse((cx - 1, cy - 1, cx + 1, cy + 1), fill=(255, 255, 255, 60))

    # symbol
    try:
        fnt = ImageFont.truetype("/usr/share/fonts/truetype/ubuntu/Ubuntu-Bold.ttf", 22)
    except:
        fnt = ImageFont.load_default()
    ltr = {"healthy": "✓", "degraded": "!", "down": "✗", "syncing": "⟳", "unknown": "?"}.get(status, "?")
    bbox = draw.textbbox((0, 0), ltr, font=fnt)
    tw, th = bbox[2] - bbox[0], bbox[3] - bbox[1]
    draw.text((cx - tw // 2, cy - th // 2 - 1), ltr, fill=(255, 255, 255, 255), font=fnt)

    return im


def _save_icon(status):
    path = os.path.join(ICON_DIR, f"node-{status}.png")
    _make_icon_pil(status).save(path)
    return path


def _send_tg_alert(msg):
    token_file = os.path.expanduser("~/.config/opencode-tg-bot.token")
    chat_file = os.path.expanduser("~/.config/opencode-tg-bot.chat")
    try:
        with open(token_file) as f: token = f.read().strip()
        with open(chat_file) as f: chat_id = f.read().strip()
        data = json.dumps({"chat_id": chat_id, "text": msg}).encode()
        # bypass global Tor SOCKS proxy — connect direct to Telegram
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        req = urllib.request.Request(
            f"https://api.telegram.org/bot{token}/sendMessage",
            data=data, headers={"Content-Type": "application/json"}
        )
        opener.open(req, timeout=10)
        log(f"tg alert sent: {msg[:80]}")
    except Exception as e:
        log(f"tg alert failed: {e}")


class NodeMonitor:
    _alert_cooldown = 600  # seconds between Telegram alerts

    def __init__(self):
        self.health_status = "unknown"
        self.sys = {}
        self.svcs = {}
        self.net = {}
        self.econ = {}
        self.chat = {}
        self.paranoidx = {}
        self.lock = threading.Lock()
        self._stop = False
        self._status = {"status": "unknown", "bridge": False, "healthy": False, "messages": 0}

        # auto-heal state
        self._consecutive_failures = 0
        self._is_recovering = False
        self._last_known_status = None
        self._alarm_sent = False
        self._last_alert_time = 0
        self._restart_times = []  # timestamps of restarts (for loop detection)
        self._mem_high_count = 0  # consecutive high memory samples
        self._disk_pct_history = []  # last 10 disk samples for trend

        # service auto-heal state
        self._xray_healthy = True
        self._xray_alert_sent = False
        self._bridge_healthy = True
        self._bridge_disconnect_start = None
        self._last_disk_alert_pct = 0
        self._docker_unhealthy = set()
        self._init_ui()

    def _send_tg_alert_cooldown(self, msg):
        if time.time() - self._last_alert_time < self._alert_cooldown:
            log(f"tg alert suppressed (cooldown): {msg[:60]}")
            return
        self._last_alert_time = time.time()
        _send_tg_alert(msg)

    def _init_ui(self):
        # pre-generate icon files
        for s in ("healthy", "degraded", "down", "syncing", "unknown"):
            _save_icon(s)

        self._build_indicator()
        self._build_window()
        self._update_indicator()
        self.poller = threading.Thread(target=self._poller_loop, daemon=True)
        self.poller.start()
        GLib.timeout_add(1000, self._tick)

    def _tick(self):
        if self._stop:
            Gtk.main_quit()
            return False
        return True

    # ── indicator (AyatanaAppIndicator3 — works on Wayland) ────
    def _build_indicator(self):
        self.indicator = AyatanaAppIndicator3.Indicator.new(
            "simplex-node-monitor",
            "node-unknown",
            AyatanaAppIndicator3.IndicatorCategory.APPLICATION_STATUS,
        )
        self.indicator.set_icon_theme_path(ICON_DIR)
        self.indicator.set_icon_full("node-unknown", "simplex-node: connecting…")
        self.indicator.set_label("", "")
        self.indicator.set_status(AyatanaAppIndicator3.IndicatorStatus.ACTIVE)
        self._build_indicator_menu()

    def _build_indicator_menu(self):
        menu = Gtk.Menu()
        items = [
            ("🖥  Show Monitor", self._show_window),
            (None, None),
            ("▶  Start Node", self._do_start),
            ("⏹  Stop Node", self._do_stop),
            ("🔄  Restart Node", self._do_restart),
            ("🔍  Test Node", self._do_test),
            (None, None),
            ("📡  Restart xray", self._do_restart_xray),
            ("🐳  Restart Docker Stack", self._do_restart_docker),
            (None, None),
            ("📋  Show Logs", self._do_show_logs),
            (None, None),
            ("⚡  Cleanup Disk", self._do_cleanup),
            ("🧪  Self-Test", self._do_selftest),
            ("✕  Quit", self._do_quit),
        ]
        for label, cb in items:
            if label is None:
                menu.append(Gtk.SeparatorMenuItem())
            else:
                item = Gtk.MenuItem(label=label)
                item.connect("activate", lambda _, c=cb: c())
                menu.append(item)
        menu.show_all()
        self.indicator.set_menu(menu)

    def _update_indicator(self):
        name = f"node-{self.health_status}"
        path = _save_icon(self.health_status)
        label_map = {"healthy": "✓ UP", "degraded": "⚠ DEG", "down": "✗ DOWN", "syncing": "⟳ SYNC", "unknown": "?"}
        lbl = label_map.get(self.health_status, "?")
        self.indicator.set_icon_full(name, f"simplex-node: {lbl}")
        stats = self._status
        uptime = stats.get("uptime_hours", "?")
        tip = f"simplex-node: {lbl} | 📈 {uptime}h"
        if stats.get("healthy"):
            tip += " ✓"
        if stats.get("bridge"):
            tip += " | bridge ✓"
        if stats.get("messages", 0):
            tip += f" | 💬 {stats['messages']}"
        if self._is_recovering:
            tip += " | ⟳ recovering…"
        elif self._consecutive_failures > 0:
            tip += f" | ⚠ {self._consecutive_failures}x fail"
        self.indicator.set_label("", "")
        self.indicator.set_title(tip)

    # ── main window ────────────────────────────────────────────
    def _build_window(self):
        self.window = Gtk.Window(title="simplex-node Monitor")
        self.window.set_default_size(640, 480)
        self.window.set_position(Gtk.WindowPosition.CENTER)
        self.window.connect("delete-event", lambda w, e: self.window.hide() or True)

        vb = Gtk.VBox(spacing=4)
        vb.set_border_width(6)

        hdr = Gtk.Label()
        hdr.set_markup("<b>simplex-node</b>  |  <small>sequential poller · 120s cycle</small>")
        vb.pack_start(hdr, False, False, 2)

        self.notebook = Gtk.Notebook()
        self.notebook.set_tab_pos(Gtk.PositionType.TOP)
        vb.pack_start(self.notebook, True, True, 0)

        self.tab_system = self._make_tab()
        self.tab_services = self._make_tab()
        self.tab_network = self._make_tab()
        self.tab_economy = self._make_tab()
        self.tab_chat = self._make_tab()
        self.tab_paranoidx = self._make_tab()

        self.notebook.append_page(self.tab_system["scwin"], Gtk.Label(label="🖥 System"))
        self.notebook.append_page(self.tab_services["scwin"], Gtk.Label(label="⚙ Services"))
        self.notebook.append_page(self.tab_network["scwin"], Gtk.Label(label="🌐 Network"))
        self.notebook.append_page(self.tab_economy["scwin"], Gtk.Label(label="💰 Economy"))
        self.notebook.append_page(self.tab_chat["scwin"], Gtk.Label(label="💬 Chat"))
        self.notebook.append_page(self.tab_paranoidx["scwin"], Gtk.Label(label="🛡 ParanoidX"))

        self.statusbar = Gtk.Label()
        self.statusbar.set_halign(Gtk.Align.START)
        self.statusbar.set_valign(Gtk.Align.CENTER)
        self.statusbar.set_markup("<small>idle</small>")
        vb.pack_start(self.statusbar, False, False, 2)

        self.window.add(vb)

    def _make_tab(self):
        scwin = Gtk.ScrolledWindow()
        scwin.set_policy(Gtk.PolicyType.AUTOMATIC, Gtk.PolicyType.AUTOMATIC)
        vb = Gtk.VBox(spacing=2)
        vb.set_border_width(8)
        scwin.add(vb)
        return {"scwin": scwin, "vb": vb}

    def _clear_tab(self, tab):
        for ch in list(tab["vb"].get_children()):
            tab["vb"].remove(ch)

    def _show_window(self):
        self.window.set_keep_above(True)
        self.window.show_all()
        self.window.deiconify()
        self.window.present()
        self.window.grab_focus()
        GLib.timeout_add(200, lambda: self.window.set_keep_above(False)) or None

    def _toggle_window(self):
        if self.window.get_visible():
            self.window.hide()
        else:
            self._show_window()

    # ── poller (sequential — one request at a time) ────────────
    def _poller_loop(self):
        while not self._stop:
            try:
                self._poll()
            except Exception as e:
                log(f"poll error: {e}")
            for _ in range(POLL_INTERVAL):
                if self._stop:
                    return
                time.sleep(1)

    def _poll(self):
        data = {}

        # 1. health
        h = fetch_json("/api/health")
        data["_status"] = h.get("status", "unknown") if "_error" not in h else "down"
        data["_healthy"] = h.get("healthy", False)
        data["_bridge"] = h.get("bridge", False)
        self._status = h

        # 2. system (local)
        data["sys"] = system_info()

        # 3. version
        data["version"] = fetch_json("/api/version").get("version", "?")

        # 4. chat status
        cs = fetch_json("/api/chat/status")
        data["chat_status"] = cs
        data["chat_msgs"] = cs.get("message_count", 0)

        # 5. docker
        data["docker"] = fetch_json("/api/admin/docker")

        # 6. economy
        oracle = fetch_json("/api/economy/oracle")
        data["silver_price"] = oracle.get("current_price", 0)
        data["silver_updated"] = oracle.get("last_updated", "")
        data["reserve_ratio"] = fetch_json("/api/reserve/proof").get("backing_ratio", 0)

        # 7. paranoidx
        data["paranoidx"] = fetch_json("/api/paranoidx/status")

        # update
        with self.lock:
            self.sys = data["sys"]
            self.svcs = {
                "version": data["version"],
                "bridge": data["_bridge"],
                "docker": data.get("docker", {}),
                "healthy": data["_healthy"],
            }
            self.net = {"hostname": data["sys"]["hostname"], "ips": data["sys"]["ips"]}
            self.econ = {
                "silver_price": data.get("silver_price", 0),
                "silver_updated": data.get("silver_updated", ""),
                "reserve_ratio": data.get("reserve_ratio", 0),
            }
            self.chat = {
                "status": data.get("chat_status", {}),
                "message_count": data.get("chat_msgs", 0),
            }
            self.paranoidx = data.get("paranoidx", {})

            if "_error" in h:
                self.health_status = "down"
            elif not data["_healthy"]:
                self.health_status = "degraded"
            elif not data["_bridge"]:
                self.health_status = "syncing"
            else:
                self.health_status = "healthy"

            # ── auto-heal: only restart on DOWN (API unreachable) or bridge lost ──
            bridge_down = data["_healthy"] and not data["_bridge"]
            trigger_restart = self.health_status == "down" or bridge_down
            if trigger_restart:
                self._consecutive_failures += 1
                if self._last_known_status == "healthy":
                    log(f"node failing: status={self.health_status}, bridge_down={bridge_down}")
                self._last_known_status = "unhealthy"
                # restart after 3 consecutive down polls (~6min)
                if self._consecutive_failures >= 3 and not self._is_recovering:
                    self._is_recovering = True
                    reason = "API unreachable" if self.health_status == "down" else "bridge disconnected"
                    log(f"auto-heal: initiating restart ({reason})")
                    _send_tg_alert(f"⚠️ node-monitor: restarting ({reason}, {self._consecutive_failures}x)")
                    self._notify("Node Monitor", "Auto-heal: restarting node...")
                    self._do_restart_with_verify()
            elif self.health_status == "degraded":
                # degraded = server running but disk/system issues — alert but don't restart
                self._consecutive_failures = self._consecutive_failures - 1 if self._consecutive_failures > 0 else 0
                if self._last_known_status == "healthy":
                    log(f"node degraded (disk/system), no restart needed")
                    self._send_tg_alert_cooldown(f"⚠️ node-monitor: node degraded (disk/system health), monitoring...")
                self._last_known_status = "degraded"
                self._alarm_sent = False
            else:
                self._consecutive_failures = 0
                self._alarm_sent = False
                if self._last_known_status != "healthy":
                    log("node recovered")
                    _send_tg_alert("✅ node-monitor: node recovered")
                    self._notify("Node Monitor", "Node recovered")
                self._last_known_status = "healthy"

        # ── C1: xray auto-heal (only if node API is responding) ──
        xray_ok = True  # default assume healthy when node down
        if "_error" not in h and "checks" in h:
            xray_checks = [c for c in h["checks"] if c.get("name") == "xray_native"]
            xray_ok = any(c.get("status") == "ok" for c in xray_checks) if xray_checks else True
        if not xray_ok and not self._xray_alert_sent:
            log("xray down, attempting restart…")
            self._restart_xray()
            self._xray_alert_sent = True
        elif xray_ok:
            self._xray_alert_sent = False

        # ── C3: bridge auto-heal (disconnected >5 min) ──────────
        if not data["_bridge"] and data["_healthy"]:
            if self._bridge_disconnect_start is None:
                self._bridge_disconnect_start = time.time()
            elif time.time() - self._bridge_disconnect_start > 300:  # 5 min
                log("bridge disconnected >5min, restarting node…")
                _send_tg_alert("⚠️ node-monitor: bridge disconnected >5min, restarting node")
                self._notify("Node Monitor", "Bridge down >5min, restarting…")
                self._do_restart_with_verify()
                self._bridge_disconnect_start = None
        else:
            self._bridge_disconnect_start = None

        # ── C4: disk alert >90% ──────────────────────────────────
        disk_pct = data["sys"].get("disk_percent", 0)
        if disk_pct > 90 and disk_pct - self._last_disk_alert_pct > 2:
            self._last_disk_alert_pct = disk_pct
            _send_tg_alert(f"⚠️ node-monitor: disk at {disk_pct:.0f}% ({fmt_bytes(data['sys'].get('disk_used',0))} used)")
            GLib.idle_add(lambda: self._notify("Disk Alert", f"Disk at {disk_pct:.0f}%"))
        elif disk_pct < 85:
            self._last_disk_alert_pct = 0

        # ── I8: disk trend tracking ──────────────────────────────
        self._disk_pct_history.append(disk_pct)
        if len(self._disk_pct_history) > 10:
            self._disk_pct_history.pop(0)
        if len(self._disk_pct_history) >= 4:
            trend = self._disk_pct_history[-1] - self._disk_pct_history[0]
            rate_per_hour = trend * 6  # 10min interval * 6 = 1h
            if rate_per_hour > 2:
                _send_tg_alert(f"⚠️ node-monitor: disk growing {rate_per_hour:.1f}%/hour ({disk_pct:.0f}%)")

        # ── I8: memory threshold check ──────────────────────────
        mem_pct = data["sys"].get("mem_percent", 0)
        if mem_pct > 90:
            self._mem_high_count += 1
            if self._mem_high_count >= 3:  # 3 consecutive checks (~30min)
                _send_tg_alert(f"⚠️ node-monitor: memory >90% for {self._mem_high_count} checks ({mem_pct:.0f}%)")
        else:
            self._mem_high_count = 0

        # ── C2: docker auto-heal ─────────────────────────────────
        svcStatus = fetch_json("/api/admin/service/status")
        if "_error" not in svcStatus:
            for name, status in svcStatus.get("containers", {}).items():
                if "Unhealthy" in status or "exited" in status.lower():
                    if name not in self._docker_unhealthy:
                        log(f"docker {name} unhealthy ({status}), restarting…")
                        _send_tg_alert(f"⚠️ docker {name} unhealthy, restarting")
                        subprocess.run(["docker", "restart", name], capture_output=True)
                        self._docker_unhealthy.add(name)
                else:
                    self._docker_unhealthy.discard(name)

        GLib.idle_add(self._refresh_ui)

    def _refresh_ui(self):
        if self._stop:
            return False
        self._render_tab_system()
        self._render_tab_services()
        self._render_tab_network()
        self._render_tab_economy()
        self._render_tab_chat()
        self._render_tab_paranoidx()
        self._update_indicator()
        heal = ""
        if self._is_recovering:
            heal = " | 🔄 recovering…"
        elif self._consecutive_failures > 1:
            heal = f" | ⚠ {self._consecutive_failures}x fail, restart queued"
        lbl = {"healthy": "✓ UP", "degraded": "⚠ DEG", "down": "✗ DOWN", "syncing": "⟳ SYNC", "unknown": "?"}
        self.statusbar.set_markup(
            f"<small>last poll: {time.strftime('%H:%M:%S')}  |  "
            f"status: {lbl.get(self.health_status, '?')}{heal}  |  "
            f"poll: {POLL_INTERVAL}s</small>"
        )
        return False

    # ── tab renderers ──────────────────────────────────────────
    def _label(self, parent, text, markup=False):
        l = Gtk.Label()
        l.set_halign(Gtk.Align.START)
        l.set_valign(Gtk.Align.CENTER)
        l.set_line_wrap(True)
        if markup:
            l.set_markup(text)
        else:
            l.set_text(text)
        parent.pack_start(l, False, False, 1)
        return l

    def _render_tab_system(self):
        with self.lock:
            s = dict(self.sys)
        vb = self.tab_system["vb"]
        self._clear_tab(self.tab_system)
        if not s:
            self._label(vb, "No data (poller starting…)")
            vb.show_all()
            return
        self._label(vb, f"<b>CPU:</b> {s.get('cpu',0):.1f}%", markup=True)
        mem = f"{fmt_bytes(s.get('mem_used',0))} / {fmt_bytes(s.get('mem_total',0))} ({s.get('mem_percent',0):.1f}%)"
        self._label(vb, f"<b>RAM:</b> {mem}", markup=True)
        disk = f"{fmt_bytes(s.get('disk_used',0))} / {fmt_bytes(s.get('disk_total',0))} ({s.get('disk_percent',0):.1f}%)"
        self._label(vb, f"<b>Disk:</b> {disk}", markup=True)
        self._label(vb, f"<b>Uptime:</b> {fmt_uptime(s.get('uptime',0))}", markup=True)
        self._label(vb, f"<b>Host:</b> {s.get('hostname','?')}", markup=True)
        vb.show_all()

    def _render_tab_services(self):
        with self.lock:
            sv = dict(self.svcs)
        vb = self.tab_services["vb"]
        self._clear_tab(self.tab_services)
        if not sv:
            self._label(vb, "No data…")
            vb.show_all()
            return
        self._label(vb, f"<b>Version:</b> {sv.get('version','?')}", markup=True)
        ok = sv.get("healthy", False)
        self._label(vb, f"<b>Health:</b> {'✓ healthy' if ok else '⚠ degraded'}", markup=True)
        br = sv.get("bridge", False)
        self._label(vb, f"<b>Bridge:</b> {'✓ connected' if br else '✗ disconnected'}", markup=True)
        dk = sv.get("docker", {})
        if isinstance(dk, dict):
            containers = dk.get("containers", dk.get("services", []))
            if isinstance(containers, list):
                for c in containers:
                    if isinstance(c, dict):
                        nm = c.get("name", c.get("service", "?"))
                        st = c.get("status", c.get("state", "?"))
                        icon = "✅" if "healthy" in str(st).lower() or "up" in str(st).lower() else "⚠️"
                        self._label(vb, f"  {icon} {nm}: {st}")
            elif isinstance(containers, dict):
                for nm, st in containers.items():
                    icon = "✅" if "healthy" in str(st).lower() or "up" in str(st).lower() else "⚠️"
                    self._label(vb, f"  {icon} {nm}: {st}")
        # Fetch per-container status from /api/admin/service/status
        svcStatus = fetch_json("/api/admin/service/status")
        if "_error" not in svcStatus:
            self._label(vb, "", markup=False)
            self._label(vb, "<b>Containers (docker compose):</b>", markup=True)
            for name, status in svcStatus.get("containers", {}).items():
                short = name.replace("simplex-node-", "")
                icon = "✅" if "Up" in status else "❌"
                self._label(vb, f"  {icon} {short}: {status}")
        vb.show_all()

    def _render_tab_network(self):
        with self.lock:
            n = dict(self.net)
        vb = self.tab_network["vb"]
        self._clear_tab(self.tab_network)
        if not n:
            self._label(vb, "No data…")
            vb.show_all()
            return
        self._label(vb, f"<b>Hostname:</b> {n.get('hostname','?')}", markup=True)
        for ip in n.get("ips", []):
            self._label(vb, f"  {ip}")
        self._label(vb, f"<b>API:</b> {API}", markup=True)
        vb.show_all()

    def _render_tab_economy(self):
        with self.lock:
            e = dict(self.econ)
        vb = self.tab_economy["vb"]
        self._clear_tab(self.tab_economy)
        if not e:
            self._label(vb, "No data…")
            vb.show_all()
            return
        price = e.get("silver_price", 0)
        updated = e.get("silver_updated", "")
        self._label(vb, f"<b>Silver spot:</b> ${price:.2f}/oz", markup=True)
        if updated:
            self._label(vb, f"<b>Updated:</b> {updated}")
        self._label(vb, f"<b>Source:</b> gold-api.com → Swissquote")
        rr = e.get("reserve_ratio", 0)
        if isinstance(rr, (int, float)) and rr > 0:
            self._label(vb, f"<b>Reserve ratio:</b> {rr:,.0f}:1", markup=True)
        vb.show_all()

    def _render_tab_chat(self):
        with self.lock:
            c = dict(self.chat)
        vb = self.tab_chat["vb"]
        self._clear_tab(self.tab_chat)
        if not c:
            self._label(vb, "No data…")
            vb.show_all()
            return
        mc = c.get("message_count", 0)
        self._label(vb, f"<b>Messages:</b> {mc}", markup=True)
        cs = c.get("status", {})
        if isinstance(cs, dict):
            for k, v in cs.items():
                if k != "_error":
                    self._label(vb, f"  {k}: {v}")
        vb.show_all()

    def _render_tab_paranoidx(self):
        with self.lock:
            p = dict(self.paranoidx)
        vb = self.tab_paranoidx["vb"]
        self._clear_tab(self.tab_paranoidx)
        if not p or "_error" in p:
            self._label(vb, "No ParanoidX data")
            vb.show_all()
            return
        overall = p.get("overall_healthy", False)
        icon = "✅" if overall else "⚠️"
        self._label(vb, f"<b>{icon} Overall:</b> {'Healthy' if overall else 'Degraded'}", markup=True)
        self._label(vb, f"<b>Updated:</b> {p.get('last_updated','?')}", markup=True)
        self._label(vb, "")
        layers = p.get("layers", [])
        for layer in layers:
            name = layer.get("layer", "?")
            healthy = layer.get("healthy", False)
            lat = layer.get("latency_ms", 0)
            msg = layer.get("message", "")
            licon = "✅" if healthy else "❌"
            self._label(vb, f"{licon} <b>{name}</b>", markup=True)
            self._label(vb, f"    latency: {lat}ms")
            self._label(vb, f"    {msg}")
        vb.show_all()

    # ── actions ────────────────────────────────────────────────
    def _do_start(self):
        def run():
            log("starting node via launch-node.sh…")
            launch = os.path.expanduser("~/simplex-node/scripts/launch-node.sh")
            if os.path.isfile(launch):
                subprocess.Popen(["bash", launch], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, preexec_fn=os.setpgrp)
                GLib.idle_add(lambda: self._notify("simplex-node", "launch-node.sh started"))
            else:
                subprocess.Popen(["nohup", NODE_BIN], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, preexec_fn=os.setpgrp)
                GLib.idle_add(lambda: self._notify("simplex-node", "start command sent (direct)"))
        threading.Thread(target=run, daemon=True).start()

    def _do_stop(self):
        def run():
            log("stopping node…")
            subprocess.run(["pkill", "-f", "^/home/tomas/bin/simplex-node"], capture_output=True)
            GLib.idle_add(lambda: self._notify("simplex-node", "stop command sent"))
        threading.Thread(target=run, daemon=True).start()

    def _do_restart(self):
        def run():
            log("restarting node via systemctl…")
            subprocess.run(["systemctl", "--user", "restart", "simplex-node.service"], capture_output=True)
            GLib.idle_add(lambda: self._notify("simplex-node", "restarted via systemctl"))
        threading.Thread(target=run, daemon=True).start()

    def _do_restart_with_verify(self):
        # Track restart for loop detection
        now = time.time()
        self._restart_times.append(now)
        self._restart_times = [t for t in self._restart_times if now - t < 3600]
        recent = len(self._restart_times)
        if recent > 3:
            log(f"⚠️ RESTART LOOP: {recent} restarts in last hour — pausing auto-heal")
            _send_tg_alert(f"⚠️ RESTART LOOP detected: {recent} restarts in 1h, pausing auto-heal for 30min")
            self._is_recovering = False
            self._consecutive_failures = 0
            time.sleep(1800)
            return
        # Check maintenance mode touch-file
        maint_file = os.path.join(os.environ.get("DATA_DIR", os.path.expanduser("~/.local/share/simplex-node")), ".maintenance")
        if os.path.isfile(maint_file) and (time.time() - os.path.getmtime(maint_file)) < 3600:
            log("auto-heal: maintenance mode active, skipping restart")
            self._is_recovering = False
            self._consecutive_failures = max(0, self._consecutive_failures - 1)
            return
        def run():
            log("auto-restart: restarting via systemctl…")
            subprocess.run(["systemctl", "--user", "restart", "simplex-node.service"], capture_output=True, timeout=30)
            GLib.idle_add(lambda: self._notify("simplex-node", "auto-restart initiated"))
            time.sleep(10)
            self._verify_recovery()
        threading.Thread(target=run, daemon=True).start()

    def _verify_recovery(self):
        deadline = time.time() + 90
        ok = False
        while time.time() < deadline:
            time.sleep(5)
            h = fetch_json("/api/health")
            if "_error" not in h and h.get("healthy", False):
                ok = True
                break
        if ok:
            log("auto-heal: recovery verified")
            self._send_tg_alert_cooldown("✅ node-monitor: node recovered after auto-restart")
            GLib.idle_add(lambda: self._notify("simplex-node", "node recovered after restart"))
        else:
            log("auto-heal: recovery FAILED")
            self._send_tg_alert_cooldown("🚨 node-monitor: node DOWN after restart — manual intervention needed!")
            GLib.idle_add(lambda: self._notify("simplex-node", "🚨 RECOVERY FAILED"))
        self._is_recovering = False
        self._consecutive_failures = 0

    def _do_test(self):
        def run():
            log("testing node…")
            result = fetch_json("/api/health")
            msg = json.dumps(result, indent=2, ensure_ascii=False)
            GLib.idle_add(lambda: self._show_dialog("Test Result", msg))
        threading.Thread(target=run, daemon=True).start()

    def _restart_xray(self):
        log("restarting xray…")
        subprocess.run(["pkill", "-x", "xray"], capture_output=True)
        time.sleep(1)
        xray_bin = os.path.expanduser("~/bin/v2ray/xray")
        xray_cfg = os.path.expanduser("~/bin/v2ray/config.json")
        if os.path.isfile(xray_bin) and os.path.isfile(xray_cfg):
            subprocess.Popen(
                ["nohup", xray_bin, "run", "-c", xray_cfg],
                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, preexec_fn=os.setpgrp
            )
            GLib.idle_add(lambda: self._notify("simplex-node", "xray restart initiated"))
        else:
            GLib.idle_add(lambda: self._notify("simplex-node", "xray binary/config not found"))

    def _do_restart_xray(self):
        threading.Thread(target=self._restart_xray, daemon=True).start()

    def _do_restart_docker(self):
        def run():
            log("restarting docker stack…")
            compose_dir = os.path.expanduser("~/simplex-node/docker")
            if os.path.isdir(compose_dir):
                subprocess.run(["docker", "compose", "-f", os.path.join(compose_dir, "docker-compose.yml"), "restart"],
                               cwd=compose_dir, capture_output=True)
                GLib.idle_add(lambda: self._notify("simplex-node", "docker stack restarted"))
            else:
                GLib.idle_add(lambda: self._notify("simplex-node", "docker compose dir not found"))
        threading.Thread(target=run, daemon=True).start()

    def _do_show_logs(self):
        def run():
            log_dir = os.path.expanduser("~/.local/share/simplex-node/logs")
            if os.path.isdir(log_dir):
                recent = subprocess.run(
                    ["bash", "-c", f"tail -100 {log_dir}/dashboard.log {log_dir}/xray.log {log_dir}/island-bot-setup.log 2>/dev/null"],
                    capture_output=True, text=True
                ).stdout
                GLib.idle_add(lambda: self._show_dialog("Logs (tail -100)", recent or "(empty)"))
            else:
                GLib.idle_add(lambda: self._show_dialog("Logs", f"Log dir not found: {log_dir}"))
        threading.Thread(target=run, daemon=True).start()

    def _do_selftest(self):
        def run():
            log("running self-test…")
            endpoints = [
                ("api/version", "/api/version"),
                ("api/health", "/api/health"),
                ("api/health/checks", "/api/health/checks"),
                ("api/chat/status", "/api/chat/status"),
                ("api/admin/docker", "/api/admin/docker"),
                ("api/admin/service/status", "/api/admin/service/status"),
                ("api/economy/oracle", "/api/economy/oracle"),
                ("api/paranoidx/status", "/api/paranoidx/status"),
            ]
            results = []
            ok = 0
            fail = 0
            for name, path in endpoints:
                data = fetch_json(path)
                if "_error" in data:
                    results.append(f"❌ {name}: {data['_error']}")
                    fail += 1
                else:
                    results.append(f"✅ {name}: OK")
                    ok += 1
            msg = f"📊 Self-Test: {ok}/{ok+fail} passed\n\n" + "\n".join(results)
            GLib.idle_add(lambda: self._show_dialog("Self-Test Results", msg))
        threading.Thread(target=run, daemon=True).start()

    def _do_cleanup(self):
        def run():
            log("running disk cleanup…")
            result = fetch_json("/api/admin/disk-cleanup")
            msg = json.dumps(result, indent=2, ensure_ascii=False)
            GLib.idle_add(lambda: self._show_dialog("Disk Cleanup Result", msg))
        threading.Thread(target=run, daemon=True).start()

    def _notify(self, title, text):
        if _HAVE_NOTIFY:
            try:
                notification = Notify.Notification.new(title, text, "dialog-information")
                notification.show()
                return
            except Exception:
                pass
        subprocess.Popen(["notify-send", title, text])

    def _show_dialog(self, title, text):
        d = Gtk.Dialog(title=title, parent=self.window, modal=True)
        d.set_default_size(500, 400)
        sw = Gtk.ScrolledWindow()
        sw.set_policy(Gtk.PolicyType.AUTOMATIC, Gtk.PolicyType.AUTOMATIC)
        l = Gtk.Label(label=text)
        l.set_selectable(True)
        l.set_line_wrap(True)
        sw.add(l)
        d.vbox.pack_start(sw, True, True, 4)
        d.add_button("Close", Gtk.ResponseType.CLOSE)
        d.show_all()
        d.run()
        d.destroy()

    def _do_quit(self):
        self._stop = True
        Gtk.main_quit()

    def run(self):
        Gtk.main()


if __name__ == "__main__":
    NodeMonitor().run()
