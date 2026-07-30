#!/usr/bin/env python3
"""Admin Telegram bridge — full node control + AI evolution bridge.

Commands:
  /status       — full node status
  /treasury     — treasury state, reserve, banknotes, RWA
  /silver       — silver reserve
  /disk         — disk usage
  /disk_check   — run disk check + alerts
  /logs [n]     — last n lines of node log
  /ps           — process list (node, bots, docker)
  /docker       — docker compose ps
  /launch       — start node
  /kill         — stop node
  /restart      — restart node cleanly
  /rebuild      — rebuild Go binary
  /vault_list   — list vault files
  /channels     — list channels
  /market       — market overview
  /backup       — trigger backup
  /test         — run test-royal.sh
  /show_context — recent prompt log
  /version      — bot + node version
  /help         — this menu

Any other text → sent to AI (Kilo) for analysis/evolution ideas.
"""

from __future__ import annotations

import os
import subprocess
import sys
import time
from pathlib import Path

# Add scripts dir to path for bot_framework import
_SCRIPTS = Path('/home/tomas/ParanoidX/scripts')
if str(_SCRIPTS) not in sys.path:
    sys.path.insert(0, str(_SCRIPTS))

from bot_framework import BotBase, BotConfig


HOME = Path('/home/tomas')
DATA = HOME / '.local' / 'share' / 'simplex-node'
SCRIPTS = HOME / 'simplex-node' / 'scripts'
PROJECT = HOME / 'simplex-node'

CONFIG = BotConfig(
    name='admin-bridge',
    token_file=HOME / '.config' / 'opencode-tg-bot.token',
    chat_file=HOME / '.config' / 'opencode-tg-bot.chat',
    offset_file=DATA / 'admin_offset.txt',
    session_file=DATA / 'kilo_session_id.txt',
    prompt_log=DATA / 'bot_full_prompt.log',
    bridge_log=DATA / 'admin_bridge.log',
    send_script=SCRIPTS / 'send-to-torquemada.sh',
)


class AdminBridge(BotBase):
    """Full node control bot — admin commands + AI evolution bridge."""

    def help_text(self) -> str:
        return (
            '⚙️ Admin Bridge — node control + AI evolution\n\n'
            '/status, /treasury, /silver, /disk, /disk_check\n'
            '/logs [n], /ps, /docker, /version\n'
            '/launch, /kill, /restart, /rebuild\n'
            '/vault_list, /channels, /market, /backup\n'
            '/test, /show_context, /help\n\n'
            'All other text → Kilo AI for analysis & evolution ideas.'
        )

    # ── shared helpers ─────────────────────────────────

    def _fmt_ok(self, data: dict | None, *keys: str) -> str:
        if data is None:
            return '(unreachable)'
        parts = []
        for k in keys:
            v = data.get(k, '?')
            parts.append(f'{k}: {v}')
        return ', '.join(parts)

    def _fmt_disk(self, disk: dict) -> list[str]:
        if not disk:
            return ['(disk data unavailable)']
        return [
            f"root: {disk.get('root_used_pct', '?')}% used, {disk.get('root_avail_mb', 0)} MB free",
            f"vault: {disk.get('vault_user_mb', 0)} MB user / {disk.get('vault_total_du_mb', 0)} MB total",
            f"data dir: {disk.get('data_dir_mb', '?')} MB",
            f"backups: {disk.get('backups_count', 0)} dirs, {disk.get('backups_total_mb', 0)} MB",
        ]

    def _fmt_node_ok(self) -> str:
        d = self.node_api_get('/api/status')
        if d is None:
            return '❌ Node unreachable'
        disk = d.get('disk', {})
        return (
            f"✅ Node: {d.get('status')}, royal={d.get('is_royal')}\n"
            f"uptime: {d.get('uptime_seconds', 0)}s\n"
            f"vault: {d.get('vault', {}).get('file_count', 0)} files, "
            f"{d.get('vault', {}).get('used_mb', 0):.1f} MB\n"
            + '\n'.join(self._fmt_disk(disk))
        )

    def _fmt_docker(self) -> str:
        out = self.run_shell(['docker', 'compose', '-f', str(PROJECT / 'docker/docker-compose.yml'), 'ps', '--format', 'table'])
        dock = self.node_api_get('/api/status')
        if dock:
            smp = dock.get('smp', {}).get('status', '?')
            xftp = dock.get('xftp', {}).get('status', '?')
            return f'SMP: {smp} | XFTP: {xftp}\n{out}'
        return out or '(docker not available)'

    # ── command dispatch ───────────────────────────────

    def handle_command(self, cmd: str) -> str:
        lower = cmd.lower().strip()

        # ── help ───────────────────────────────────────
        if lower in ('/start', '/help', 'help'):
            return self.help_text()

        # ── status ─────────────────────────────────────
        if lower == '/status':
            return self._fmt_node_ok()

        # ── treasury ───────────────────────────────────
        if lower == '/treasury':
            d = self.node_api_get('/api/treasury/state')
            if d is None:
                return '❌ Treasury unavailable'
            reserve = d.get('current_reserve_ng', 0)
            bns = d.get('banknotes', [])
            rwas = d.get('rwa', [])
            return (
                f"🏦 Treasury\n"
                f"reserve: {reserve} ng = {reserve/1e9:.3f} g = {reserve/31103480000:.4f} oz\n"
                f"banknotes: {len(bns)}\n"
                f"RWA: {len(rwas)}\n"
                f"deposits: {d.get('deposits_count', 0)}"
            )

        # ── silver ─────────────────────────────────────
        if lower == '/silver':
            ng_str = self.read_text(DATA / 'silver_reserve_ng.txt') or '0'
            try:
                ng = int(ng_str)
            except ValueError:
                ng = 0
            return (
                f"🥈 Silver reserve\n"
                f"{ng} ng = {ng/1e9:.3f} g = {ng/31103480000:.4f} oz"
            )

        # ── disk ───────────────────────────────────────
        if lower == '/disk':
            d = self.node_api_get('/api/status')
            if d is None:
                return '❌ Node unreachable'
            disk = d.get('disk', {})
            return '💾 Disk\n' + '\n'.join(self._fmt_disk(disk))

        if lower == '/disk_check':
            d = self.node_api_post('/api/disk-check')
            if d is None:
                return '❌ Disk check failed (node down?)'
            alerts = d.get('alerts') or []
            lines = ['🔍 Disk check']
            if alerts:
                lines.append('⚠️ ALERTS:')
                for a in alerts:
                    lines.append(f'  - {a}')
            else:
                lines.append('✅ All clear')
            disk = d.get('disk', {})
            lines.append(f"root avail: {disk.get('root_avail_mb', '?')} MB")
            lines.append(f"vault user: {disk.get('vault_user_mb', 0)} MB")
            return '\n'.join(lines)

        # ── logs ───────────────────────────────────────
        if lower.startswith('/logs'):
            parts = cmd.split()
            n = 20
            if len(parts) > 1:
                try:
                    n = int(parts[1])
                except ValueError:
                    pass
            n = max(5, min(n, 200))
            log_file = DATA / 'logs/dashboard.log'
            if not log_file.exists():
                return '(dashboard.log not found)'
            try:
                lines = log_file.read_text(encoding='utf-8').splitlines()
                tail = lines[-n:]
                return '📋 Last {} lines:\n{}'.format(n, '\n'.join(tail))
            except Exception as exc:
                return f'(read error: {exc})'

        # ── processes ──────────────────────────────────
        if lower == '/ps':
            out = self.run_shell(['ps', 'aux', '--sort=-%mem'])
            # filter to relevant processes
            lines = [l for l in out.split('\n') if any(x in l.lower() for x in
                ['simplex', 'python3', 'docker', 'kilo', 'telegram', 'chat'])]
            return '🔄 Processes:\n' + '\n'.join(lines[:30]) or '(none found)'

        # ── docker ─────────────────────────────────────
        if lower == '/docker':
            return '🐳 Docker:\n' + self._fmt_docker()

        # ── version ────────────────────────────────────
        if lower == '/version':
            v = self.read_text(PROJECT / 'VERSION-A1') or '(unknown)'
            bot_ver = 'admin-bridge v2 (bot_framework)'
            return f"📌 Node: {v}\n🤖 Bot: {bot_ver}"

        # ── launch / kill / restart ────────────────────
        if lower == '/launch':
            subprocess.Popen([str(SCRIPTS / 'launch-node.sh')])
            return '🚀 Node launch initiated (check /status in a few seconds)'

        if lower == '/kill':
            self.run_shell(['pkill', '-x', 'simplex-node'])
            return '⏹ Node stopped'

        if lower == '/restart':
            self.run_shell(['pkill', '-x', 'simplex-node'])
            subprocess.Popen([str(SCRIPTS / 'launch-node.sh')])
            return '🔄 Restart initiated'

        # ── rebuild ────────────────────────────────────
        if lower == '/rebuild':
            result = self.run_shell(
                ['go', 'build', '-o', str(HOME / 'bin/simplex-node'),
                 './cmd/simplex-node'],
                timeout=120,
            )
            if 'error' in result.lower() or result.startswith('('):
                return f'❌ Build failed:\n{result}'
            return f'✅ Build OK\n{result}'

        # ── vault list ─────────────────────────────────
        if lower == '/vault_list':
            d = self.node_api_get('/api/vault/list')
            if d is None:
                return '❌ Vault unavailable'
            files = d.get('files', [])
            used = d.get('used_mb', 0)
            quota = d.get('quota_mb', 2048)
            if not files:
                return f'📁 Vault: empty ({used:.1f}/{quota} MB)'
            lines = [f'📁 Vault ({used:.1f}/{quota} MB, {len(files)} files):']
            for f in files[:30]:
                lines.append(f"  {f.get('name')} ({f.get('size', 0)}B)")
            return '\n'.join(lines)

        # ── channels ───────────────────────────────────
        if lower == '/channels':
            d = self.node_api_get('/api/channels/list')
            if d is None:
                return '❌ Channels unavailable'
            channels = d if isinstance(d, list) else d.get('channels', [])
            if not channels:
                return '📡 No channels'
            lines = ['📡 Channels:']
            for ch in channels[:20]:
                name = ch.get('name', '?')
                price = ch.get('price_ng', 0)
                per_view = ch.get('per_view', False)
                tag = '👁' if per_view else '🔓'
                lines.append(f'  {tag} {name} — {price} ng')
            return '\n'.join(lines)

        # ── market ─────────────────────────────────────
        if lower == '/market':
            d = self.node_api_get('/api/market/list')
            if d is None:
                return '❌ Market unavailable'
            listings = d if isinstance(d, list) else d.get('listings', [])
            if not listings:
                return '🏪 Market: no listings'
            lines = ['🏪 Market listings:']
            for item in listings[:15]:
                serial = item.get('serial', '?')
                price = item.get('price_ng', '?')
                seller = item.get('seller', '?')
                lines.append(f'  #{serial} — {price} ng (seller: {seller})')
            return '\n'.join(lines)

        # ── backup ─────────────────────────────────────
        if lower == '/backup':
            out = self.run_shell([str(SCRIPTS / 'backup-to-usb.sh')], timeout=300)
            return f'💾 Backup result:\n{out[:2000]}'

        # ── test ───────────────────────────────────────
        if lower == '/test':
            out = self.run_shell(
                [str(SCRIPTS / 'test-royal.sh')],
                timeout=120,
            )
            return f'🧪 Test:\n{out[-2000:]}'

        # ── show_context ───────────────────────────────
        if lower == '/show_context':
            log_file = self.config.prompt_log
            if log_file and log_file.exists():
                lines = log_file.read_text(encoding='utf-8').splitlines()
                return '\n'.join(lines[-30:]) or '(empty log)'
            return '(no log yet)'

        return ''  # pass to AI


if __name__ == '__main__':
    sys.exit(AdminBridge(CONFIG).run())
