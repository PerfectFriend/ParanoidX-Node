#!/usr/bin/env python3
"""Royal public-services Telegram bot — Island services menu for citizens.

Commands:
  /status     — basic node status
  /treasury   — treasury state, reserve, banknotes
  /market     — marketplace listings
  /auctions   — active auctions
  /radio      — latest radio announcements
  /channels   — available channels
  /vault      — vault usage stats
  /ice        — ICE/TURN config for WebRTC calls
  /help       — this menu

Any other text → AI assistant (public session).
"""

from __future__ import annotations

import sys
from pathlib import Path

_SCRIPTS = Path('/home/tomas/ParanoidX/scripts')
if str(_SCRIPTS) not in sys.path:
    sys.path.insert(0, str(_SCRIPTS))

from bot_framework import BotBase, BotConfig


HOME = Path('/home/tomas')
DATA = HOME / '.local' / 'share' / 'simplex-node'
SCRIPTS = HOME / 'simplex-node' / 'scripts'
PROJECT = HOME / 'simplex-node'

CONFIG = BotConfig(
    name='royal-bot',
    token_file=HOME / '.config' / 'royal-bot.token',
    chat_file=HOME / '.config' / 'royal-bot.chat',
    offset_file=DATA / 'royal_offset.txt',
    session_file=DATA / 'royal_session_id.txt',
    prompt_log=DATA / 'bot_full_prompt.log',
    bridge_log=DATA / 'royal_bot.log',
    send_script=SCRIPTS / 'send-to-torquemada.sh',
)


class RoyalBot(BotBase):
    """Public-facing services menu bot for Island citizens."""

    def help_text(self) -> str:
        return (
            '👑 Royal Island Services\n\n'
            '/status    — node status\n'
            '/treasury  — treasury, reserve, banknotes\n'
            '/market    — marketplace listings\n'
            '/auctions  — active auctions\n'
            '/radio     — radio announcements\n'
            '/channels  — available channels\n'
            '/vault     — vault usage\n'
            '/ice       — WebRTC ICE/TURN config\n'
            '/help      — this menu\n\n'
            'Plain text → AI assistant.'
        )

    def handle_command(self, cmd: str) -> str:
        lower = cmd.lower().strip()

        if lower in ('/start', '/help', 'help'):
            return self.help_text()

        if lower == '/status':
            d = self.node_api_get('/api/status')
            if d is None:
                return '❌ Node unreachable'
            return (
                f"📊 Node status\n"
                f"status: {d.get('status')}\n"
                f"royal: {d.get('is_royal')}\n"
                f"uptime: {d.get('uptime_seconds', 0)}s\n"
                f"SMP: {d.get('smp', {}).get('status', '?')}\n"
                f"XFTP: {d.get('xftp', {}).get('status', '?')}"
            )

        if lower == '/treasury':
            d = self.node_api_get('/api/treasury/state')
            if d is None:
                return '❌ Treasury unavailable'
            reserve = d.get('current_reserve_ng', 0)
            bns = d.get('banknotes', [])
            rwas = d.get('rwa', [])
            return (
                f"🏦 Treasury\n"
                f"reserve: {reserve} ng\n"
                f"  = {reserve/1e9:.3f} g\n"
                f"  = {reserve/31103480000:.4f} oz\n"
                f"banknotes: {len(bns)}\n"
                f"RWA: {len(rwas)}"
            )

        if lower == '/market':
            d = self.node_api_get('/api/market/list')
            if d is None:
                return '❌ Market unavailable'
            listings = d if isinstance(d, list) else d.get('listings', [])
            if not listings:
                return '🏪 Market: no active listings'
            lines = ['🏪 Market listings:']
            for item in listings[:10]:
                serial = item.get('serial', '?')
                price = item.get('price_ng', '?')
                lines.append(f'  #{serial} — {price} ng')
            return '\n'.join(lines)

        if lower == '/auctions':
            d = self.node_api_get('/api/auction/active')
            if d is None:
                return '❌ Auctions unavailable'
            auctions = d if isinstance(d, list) else d.get('auctions', [])
            if not auctions:
                return '🔨 No active auctions'
            lines = ['🔨 Active auctions:']
            for a in auctions[:10]:
                aid = a.get('id', '?')
                price = a.get('current_bid', a.get('reserve_price', '?'))
                lines.append(f'  #{aid} — bid: {price} ng')
            return '\n'.join(lines)

        if lower == '/radio':
            d = self.node_api_get('/api/radio/list')
            if d is None:
                return '❌ Radio unavailable'
            items = d if isinstance(d, list) else d.get('tracks', d.get('items', []))
            if not items:
                return '📻 Radio: empty'
            lines = ['📻 Radio:']
            for item in items[:10]:
                title = item.get('title') or item.get('name') or str(item)
                lines.append(f'  • {title}')
            return '\n'.join(lines)

        if lower == '/channels':
            d = self.node_api_get('/api/channels/list')
            if d is None:
                return '❌ Channels unavailable'
            channels = d if isinstance(d, list) else d.get('channels', [])
            if not channels:
                return '📡 No channels'
            lines = ['📡 Channels:']
            for ch in channels[:15]:
                name = ch.get('name', '?')
                price = ch.get('price_ng', 0)
                lines.append(f'  • {name} — {price} ng')
            return '\n'.join(lines)

        if lower == '/vault':
            d = self.node_api_get('/api/vault/list')
            if d is None:
                return '❌ Vault unavailable'
            used = d.get('used_mb', 0)
            quota = d.get('quota_mb', 2048)
            files = d.get('files', [])
            return (
                f"📁 Vault\n"
                f"used: {used:.1f} / {quota} MB\n"
                f"files: {len(files)}\n"
                f"root disk: see /status"
            )

        if lower == '/ice':
            d = self.node_api_get('/api/ice-config')
            if d is None:
                return '❌ ICE config unavailable'
            servers = d.get('iceServers', [])
            paste = d.get('pasteLines', [])
            lines = ['🌐 ICE/TURN config:']
            for s in servers:
                for url in s.get('urls', []):
                    lines.append(f'  {url}')
            if paste:
                lines.append('')
                lines.append('Paste into SimpleX > Settings > Calls:')
                for p in paste:
                    lines.append(f'  {p}')
            return '\n'.join(lines)

        return ''  # pass to AI


if __name__ == '__main__':
    sys.exit(RoyalBot(CONFIG).run())
