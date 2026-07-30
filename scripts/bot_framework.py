#!/usr/bin/env python3
"""Bot framework for ParanoidX Telegram bots.

Provides base infrastructure for polling, command dispatch, AI bridge,
offset persistence, and error handling. Each bot is a subclass with
its own command handlers and config.

Usage:
    from bot_framework import BotConfig, BotBase

    class MyBot(BotBase):
        def handle_command(self, cmd: str) -> str:
            if cmd == '/hello':
                return 'Hello!'
            return ''  # pass to AI

    MyBot(BotConfig(
        name='my-bot',
        token_file=Path('~/.config/my-bot.token'),
        chat_file=Path('~/.config/my-bot.chat'),
        offset_file=Path('~/.local/share/simplex-node/my-bot-offset.txt'),
    )).run()
"""

from __future__ import annotations

import json
import logging
import os
import queue
import signal
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.request
from dataclasses import dataclass, field
from pathlib import Path
from typing import Callable


@dataclass
class BotConfig:
    name: str
    token_file: Path
    chat_file: Path
    offset_file: Path
    session_file: Path | None = None
    prompt_log: Path | None = None
    bridge_log: Path | None = None
    data_dir: Path = Path('/home/tomas/.local/share/simplex-node')
    kilo_bin: Path = Path('/home/tomas/.kilo/bin/kilo')
    send_script: Path | None = None
    poll_interval: float = 2.0
    kilo_timeout: int = 180


class BotBase:
    """Base class for Telegram bots with polling, AI bridge, and command dispatch."""

    def __init__(self, config: BotConfig):
        self.config = config
        self.stop = threading.Event()
        self.queue: queue.Queue[str | None] = queue.Queue(maxsize=32)
        self._setup_logging()

    def _setup_logging(self):
        self.logger = logging.getLogger(self.config.name)
        self.logger.setLevel(logging.INFO)
        if self.config.bridge_log:
            fh = logging.FileHandler(self.config.bridge_log)
            fh.setFormatter(logging.Formatter('%(asctime)s %(levelname)s %(message)s'))
            self.logger.addHandler(fh)

    # ── helpers ────────────────────────────────────────

    @staticmethod
    def read_text(path: Path) -> str:
        try:
            return path.read_text(encoding='utf-8').strip()
        except Exception:
            return ''

    @staticmethod
    def write_text(path: Path, value: str) -> None:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(value, encoding='utf-8')

    # ── Telegram API ───────────────────────────────────

    def tg_request(self, method: str, payload: dict) -> dict:
        token = self.read_text(self.config.token_file)
        if not token:
            raise RuntimeError('token not found')
        data = json.dumps(payload).encode('utf-8')
        req = urllib.request.Request(
            f'https://api.telegram.org/bot{token}/{method}',
            data=data,
            headers={'Content-Type': 'application/json'},
        )
        with urllib.request.urlopen(req, timeout=25) as resp:
            return json.loads(resp.read())

    def get_updates(self, offset: str | None):
        params: dict = {'limit': 20, 'timeout': 8}
        if offset:
            params['offset'] = offset
        result = self.tg_request('getUpdates', params)
        updates = result.get('result', [])
        new_offset = None
        for u in updates:
            uid = u.get('update_id')
            if uid is not None:
                candidate = str(uid + 1)
                if new_offset is None or candidate > new_offset:
                    new_offset = candidate
        return updates, new_offset

    def send_message(self, chat_id: str, text: str) -> None:
        chunk = 4000
        parts = [text[i:i + chunk] for i in range(0, len(text), chunk)] or [text]
        for part in parts:
            self.tg_request('sendMessage', {'chat_id': int(chat_id), 'text': part})

    # ── AI bridge ──────────────────────────────────────

    def call_kilo(self, prompt: str, session_id: str | None) -> tuple[str, str | None]:
        args = [str(self.config.kilo_bin), 'run', '--pure', '--format', 'json']
        if session_id:
            args.extend(['-s', session_id])
        args.append(prompt)
        try:
            proc = subprocess.run(
                args, capture_output=True, text=True,
                timeout=self.config.kilo_timeout, check=False,
            )
        except subprocess.TimeoutExpired:
            return '(AI timeout — try shorter prompt)', session_id
        except Exception as exc:
            self.logger.exception('kilo exec failed')
            return f'(AI error: {exc})', session_id

        texts: list[str] = []
        next_session = session_id
        for line in proc.stdout.splitlines():
            line = line.strip()
            if not line:
                continue
            try:
                obj = json.loads(line)
            except json.JSONDecodeError:
                continue
            t = obj.get('type')
            if t == 'text':
                part = obj.get('part') or {}
                if part.get('type') == 'text':
                    value = (part.get('text') or '').strip()
                    if value:
                        texts.append(value)
            elif t == 'step_finish':
                break
            elif t in ('session.id', 'session.updated'):
                val = obj.get('sessionID')
                if isinstance(val, str):
                    next_session = val

        reply = ' '.join(texts).strip() or '(empty AI response)'
        return reply, next_session

    # ── Prompt log ─────────────────────────────────────

    def append_log(self, entry: str) -> None:
        if not self.config.prompt_log:
            return
        try:
            with open(self.config.prompt_log, 'a', encoding='utf-8') as f:
                f.write(entry + '\n')
        except Exception:
            pass

    # ── Override points ────────────────────────────────

    def handle_command(self, cmd: str) -> str:
        """Override for bot-specific commands.
        Return non-empty string = fast reply (skip AI).
        Return '' = pass to AI.
        """
        return ''

    def help_text(self) -> str:
        """Override for /help."""
        return 'Available commands: /help'

    def on_startup(self) -> None:
        """Called once before main loop."""

    def on_shutdown(self) -> None:
        """Called on graceful exit."""

    # ── Worker thread ──────────────────────────────────

    def _worker(self, chat_id: str):
        session_file = self.config.session_file
        session_id = self.read_text(session_file) if session_file else None
        while not self.stop.is_set():
            try:
                prompt = self.queue.get(timeout=1)
            except queue.Empty:
                continue
            if prompt is None:
                break
            self.logger.info('prompt: %s', prompt[:120])
            try:
                reply, session_id = self.call_kilo(prompt, session_id)
            except Exception as exc:
                self.logger.exception('kilo failed')
                reply = f'(bridge error: {exc})'
            self.logger.info('reply len=%d', len(reply))
            self.append_log(f'AI: {reply}')
            self.send_message(chat_id, reply)
            if session_id and session_file:
                self.write_text(session_file, session_id)

    # ── Node API helpers (for subclasses) ──────────────

    def node_api_get(self, path: str, timeout: int = 5) -> dict | None:
        """Call the local simplex-node API and return parsed JSON or None."""
        try:
            with urllib.request.urlopen(f'http://127.0.0.1:8080{path}', timeout=timeout) as r:
                return json.loads(r.read())
        except Exception as exc:
            self.logger.debug('node API %s: %s', path, exc)
            return None

    def node_api_post(self, path: str, data: dict | None = None, timeout: int = 10) -> dict | None:
        """POST to local simplex-node API."""
        try:
            payload = json.dumps(data or {}).encode('utf-8') if data else None
            req = urllib.request.Request(
                f'http://127.0.0.1:8080{path}',
                data=payload,
                headers={'Content-Type': 'application/json'} if payload else {},
            )
            with urllib.request.urlopen(req, timeout=timeout) as r:
                return json.loads(r.read())
        except Exception as exc:
            self.logger.debug('node API POST %s: %s', path, exc)
            return None

    def run_shell(self, cmd: list[str], timeout: int = 30) -> str:
        """Run a shell command and return stdout+stderr."""
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True, timeout=timeout, check=False)
            out = proc.stdout.strip()
            err = proc.stderr.strip()
            return (out + '\n' + err).strip() or '(empty output)'
        except subprocess.TimeoutExpired:
            return f'(timeout {timeout}s)'
        except Exception as exc:
            return f'(error: {exc})'

    # ── Main loop ──────────────────────────────────────

    def run(self) -> int:
        token = self.read_text(self.config.token_file)
        chat_id = self.read_text(self.config.chat_file)
        if not token:
            self.logger.error('missing token')
            return 1

        offset = self.read_text(self.config.offset_file)
        if offset and not offset.isdigit():
            offset = None

        self.logger.info('starting, offset=%s, chat_filter=%s', offset or 'fresh', chat_id or 'ALL')

        if chat_id:
            worker_thread = threading.Thread(target=self._worker, args=(chat_id,), daemon=True)
            worker_thread.start()

        def on_exit(signum, frame):
            self.stop.set()
            if chat_id:
                self.queue.put(None)
                worker_thread.join(timeout=5)
            self.on_shutdown()

        signal.signal(signal.SIGINT, on_exit)
        signal.signal(signal.SIGTERM, on_exit)

        self.on_startup()
        if not offset:
            self.logger.info('fresh start — waiting 3s for stale connections to clear')
            self.stop.wait(3)
        self.logger.info('%s started', self.config.name)

        poll_errors = 0
        while not self.stop.is_set():
            try:
                updates, new_offset = self.get_updates(offset)
                poll_errors = 0
            except urllib.error.HTTPError as exc:
                self.logger.error('poll HTTP %d: %s', exc.code, exc)
                if exc.code == 409:
                    self.logger.warning('409 conflict — waiting 30s for stale connection to expire')
                    time.sleep(30)
                else:
                    time.sleep(self.config.poll_interval * min(poll_errors + 1, 10))
                poll_errors += 1
                continue
            except Exception as exc:
                self.logger.error('poll error: %s', exc)
                backoff = min(poll_errors, 5)
                time.sleep(self.config.poll_interval * (backoff + 1))
                poll_errors += 1
                continue

            if new_offset and new_offset != offset:
                self.write_text(self.config.offset_file, new_offset)
                offset = new_offset

            for update in updates:
                msg = update.get('message') or update.get('edited_message') or {}
                uid = str(msg.get('chat', {}).get('id', ''))
                if chat_id and uid != chat_id:
                    continue
                target = uid if uid else chat_id
                text = (msg.get('text') or msg.get('caption') or '').strip()
                if not text:
                    text = '[non-text]'
                self.append_log(f"{time.strftime('%Y-%m-%d %H:%M:%S')} USER@{target}: {text}")

                reply = self.handle_command(text)
                if reply:
                    self.append_log(f'AI@{target}: {reply}')
                    self.send_message(target or chat_id, reply)
                    continue

                if text.startswith('/'):
                    self.send_message(target or chat_id, f'Unknown command: {text}')
                    continue

                if chat_id and target == chat_id:
                    try:
                        self.queue.put(text, timeout=1)
                    except queue.Full:
                        self.logger.error('queue full, dropping: %s', text[:120])

            time.sleep(self.config.poll_interval)
        return 0


def run_bot(cls: type[BotBase], config: BotConfig) -> int:
    """Convenience: instantiate and run a bot."""
    return cls(config).run()


if __name__ == '__main__':
    print("bot_framework.py — shared library, not meant to be run directly.")
    print("Import BotConfig, BotBase and subclass to create your bot.")
    sys.exit(1)
