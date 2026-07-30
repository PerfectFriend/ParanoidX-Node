#!/usr/bin/env python3
"""BOT TEMPLATE — clone this to create a new Telegram bot.

USAGE:
  1. cp bot_template.py my_new_bot.py
  2. Edit CONFIG fields (name, token_file, chat_file, offset_file)
  3. Create token file:  echo "BOT_TOKEN" > ~/.config/my-new-bot.token
  4. Create chat file:   echo "CHAT_ID" > ~/.config/my-new-bot.chat
  5. Implement handle_command() with your commands
  6. Run:  python3 my_new_bot.py

For non-Telegram protocols (SimpleX, Matrix, etc.):
  - Subclass BotBase, override tg_request/get_updates/send_message
  - The polling loop, AI bridge, offset management stay the same

See bot_framework.py for all available helpers:
  - node_api_get(path)      — GET simplex-node API
  - node_api_post(path, d)  — POST simplex-node API
  - run_shell(cmd)          — run shell command
  - send_message(chat, txt) — send reply
  - call_kilo(prompt, sid)  — AI query
"""

from __future__ import annotations

import sys
from pathlib import Path

# ── framework import ──────────────────────────────────
_SCRIPTS = Path('/home/tomas/ParanoidX/scripts')
if str(_SCRIPTS) not in sys.path:
    sys.path.insert(0, str(_SCRIPTS))

from bot_framework import BotBase, BotConfig

# ── paths (edit these) ────────────────────────────────
HOME = Path('/home/tomas')
DATA = HOME / '.local' / 'share' / 'simplex-node'
PROJECT = HOME / 'simplex-node'

CONFIG = BotConfig(
    name='my-new-bot',                     # unique bot name (used in logs)
    token_file=HOME / '.config' / 'my-new-bot.token',
    chat_file=HOME / '.config' / 'my-new-bot.chat',
    offset_file=DATA / 'my-new-bot-offset.txt',  # ← MUST be unique per bot!
    session_file=DATA / 'my-new-bot-session.txt',
    prompt_log=DATA / 'bot_full_prompt.log',
    bridge_log=DATA / 'my-new-bot.log',
)


class MyNewBot(BotBase):
    """Edit this class to add your commands."""

    def help_text(self) -> str:
        return (
            '🤖 My New Bot\n\n'
            '/help — this menu\n'
            '/ping — pong\n\n'
            'Any text → AI assistant'
        )

    def handle_command(self, cmd: str) -> str:
        lower = cmd.lower().strip()

        if lower in ('/start', '/help'):
            return self.help_text()

        if lower == '/ping':
            return 'pong'

        # ── Node API example ───────────────────────────
        if lower == '/node_status':
            d = self.node_api_get('/api/status')
            if d is None:
                return 'Node unreachable'
            return f"status: {d.get('status')}"

        # ── Shell command example ──────────────────────
        if lower == '/uptime':
            out = self.run_shell(['uptime'])
            return f'Uptime: {out}'

        # ── AI example ─────────────────────────────────
        if lower == '/story':
            # This will be passed to AI worker thread
            return ''  # return '' to pass to AI

        return ''  # pass to AI (all unknown text → AI)


if __name__ == '__main__':
    sys.exit(MyNewBot(CONFIG).run())
