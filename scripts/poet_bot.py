#!/usr/bin/env python3
"""DarkPushkin — generates SUNO AI lyrics in Russian, English, Spanish.

Send theme (style) → bot replies with 3-language song lyrics.
Uses Kilo AI via the framework's call_kilo bridge.
"""

from __future__ import annotations

import queue
import sys
import threading
import time
from pathlib import Path

_SCRIPTS = Path('/home/tomas/ParanoidX/scripts')
if str(_SCRIPTS) not in sys.path:
    sys.path.insert(0, str(_SCRIPTS))

from bot_framework import BotBase, BotConfig


HOME = Path('/home/tomas')
DATA = HOME / '.local' / 'share' / 'simplex-node'

CONFIG = BotConfig(
    name='darkpushkin',
    token_file=HOME / '.config' / 'poet-bot.token',
    chat_file=HOME / '.config' / 'poet-bot.chat',
    offset_file=DATA / 'poet_offset.txt',
    bridge_log=DATA / 'poet_bot.log',
    prompt_log=DATA / 'bot_full_prompt.log',
    kilo_timeout=300,
)

SUNO_PROMPT = """You are a songwriter for SUNO AI. Language: {lang}. Style: {style}.

Structure: [Intro] [Verse 1] [Chorus 1] [Verse 2] [Chorus 2] [Verse 3] [Chorus 3] [Outro]

Use rhyme. Theme: {theme}"""


class PoetBot(BotBase):
    """Generates poems in 3 languages via Kilo AI."""

    def help_text(self) -> str:
        return (
            'DarkPushkin — SUNO AI lyrics\n\n'
            'Send: theme (style)\n'
            '3 languages: RU + EN + ES\n\n'
            'Examples:\n'
            '  ocean (romantic ballad)\n'
            '  love (rap)\n'
            '  sunset (electronic)\n'
            '  car mechanic love (explicit)\n'
            '  digital age (march)\n\n'
            'Styles: детский, романтик, героический, '
            'марш, речитатив рэп, баллада, шансон, '
            'электроника, поп, рок, джаз, кантри, '
            'эксплицит, или любой свой'
        )

    def handle_command(self, cmd: str) -> str:
        return ''

    def on_startup(self) -> None:
        self._poem_queue: queue.Queue = queue.Queue(maxsize=16)
        self._poem_thread = threading.Thread(target=self._poem_worker, daemon=True)
        self._poem_thread.start()

    def _poem_worker(self) -> None:
        while not self.stop.is_set():
            try:
                chat_id, theme = self._poem_queue.get(timeout=1)
            except queue.Empty:
                continue
            if chat_id is None:
                break
            try:
                self._generate_and_reply(chat_id, theme)
            except Exception as exc:
                self.logger.exception('poem worker failed: %s', exc)

    MIN_LYRICS_LEN = 200

    def _generate_with_retry(self, code: str, lang: str, theme: str, style: str) -> tuple[str, float, str | None]:
        prompts = [
            SUNO_PROMPT.format(lang=lang, theme=theme, style=style),
            f'Write a rhyming song in {lang}. Style: {style}. Theme: {theme}. Structure: [Verse 1] [Chorus] [Verse 2] [Chorus] [Outro]',
        ]
        last_reply = ''
        last_elapsed = 0.0
        for attempt in range(3):
            prompt = prompts[0] if attempt == 0 else prompts[-1]
            self.logger.info('calling kilo for %s (attempt %d)', lang, attempt + 1)
            start = time.time()
            try:
                reply, _ = self.call_kilo(prompt, None)
            except Exception as exc:
                last_reply = ''
                last_elapsed = time.time() - start
                self.logger.warning('%s attempt %d exception: %s', lang, attempt + 1, exc)
                continue
            last_reply = reply
            last_elapsed = time.time() - start
            is_error = (
                reply.startswith('(Kilo timeout') or reply.startswith('(kilo error')
                or reply.startswith('(empty') or reply.startswith('(AI'))
            if is_error:
                self.logger.warning('%s attempt %d error: %s', lang, attempt + 1, reply)
                continue
            if len(reply) < self.MIN_LYRICS_LEN:
                self.logger.warning('%s too short (%d chars), retrying...', lang, len(reply))
                continue
            return reply, last_elapsed, None
        return last_reply, last_elapsed, 'failed after 3 attempts'

    def _generate_and_reply(self, chat_id: str, raw: str) -> None:
        self.logger.info('generating poems for %s: %s', chat_id, raw[:80])

        theme, style = self._parse_input(raw)

        LANGUAGES = [
            ('RU', 'Russian'),
            ('EN', 'English'),
            ('ES', 'Spanish'),
        ]

        try:
            self.send_message(chat_id,
                f'DarkPushkin working on "{theme}" ({style})...\nRU + EN + ES')
        except Exception:
            pass

        stats = []
        total_start = time.time()
        for code, lang in LANGUAGES:
            reply, elapsed, err = self._generate_with_retry(code, lang, theme, style)
            if err is not None:
                self.logger.warning('kilo failed for %s: %s', lang, err)
                stats.append((code, lang, elapsed, 0, err))
                try:
                    self.send_message(chat_id,
                        f'[{code}] {lang}\n\nSorry, generation failed: {err}')
                except Exception:
                    pass
            else:
                stats.append((code, lang, elapsed, len(reply), None))
                header = f'[{code}] {lang} — {theme} ({style})'
                body = reply if len(reply) <= 3800 else reply[:3800] + '\n...'
                try:
                    self.send_message(chat_id, f'{header}\n\n{body}')
                    self.logger.info('%s sent, len=%d, time=%.1fs', lang, len(reply), elapsed)
                except Exception as exc:
                    self.logger.error('send %s failed: %s', lang, exc)

        total_elapsed = time.time() - total_start
        ok = sum(1 for s in stats if s[4] is None)
        fail = sum(1 for s in stats if s[4] is not None)
        total_chars = sum(s[3] for s in stats)
        lines = [
            f'— {code} {lang}: {elapsed:.1f}s, {chars} chars'
            if err is None else
            f'— {code} {lang}: {elapsed:.1f}s, FAILED — {err}'
            for code, lang, elapsed, chars, err in stats
        ]
        summary = (
            f'Stats\n\n'
            f'Theme: {theme}\n'
            f'Style: {style}\n'
            f'Total time: {total_elapsed:.1f}s\n'
            f'OK: {ok} | Failed: {fail}\n'
            f'Total chars: {total_chars}\n\n' + '\n'.join(lines)
        )
        try:
            self.send_message(chat_id, summary)
            self.logger.info('summary sent for %s', chat_id)
        except Exception as exc:
            self.logger.error('send summary failed: %s', exc)

    @staticmethod
    def _parse_input(raw: str) -> tuple[str, str]:
        raw = raw.strip()
        if raw.endswith(')'):
            idx = raw.rfind('(')
            if idx > 0:
                theme = raw[:idx].strip()
                style = raw[idx + 1:-1].strip()
                if theme and style:
                    return theme, style
        return raw, 'default'

    def run(self) -> int:
        token = self.read_text(self.config.token_file)
        if not token:
            self.logger.error('missing token')
            return 1

        offset = self.read_text(self.config.offset_file)
        if offset and not offset.isdigit():
            offset = None

        chat_filter = self.read_text(self.config.chat_file)
        self.logger.info('starting, offset=%s, chat_filter=%s',
                         offset or 'fresh', chat_filter or 'ALL')

        self.on_startup()
        if not offset:
            self.logger.info('fresh start — waiting 3s')
            self.stop.wait(3)
        self.logger.info('DarkPushkin started')

        poll_errors = 0
        while not self.stop.is_set():
            try:
                updates, new_offset = self.get_updates(offset)
                poll_errors = 0
            except __import__('urllib').error.HTTPError as exc:
                self.logger.error('poll HTTP %d', exc.code)
                if exc.code == 409:
                    self.logger.warning('409 — waiting 30s')
                    self.stop.wait(30)
                else:
                    self.stop.wait(self.config.poll_interval * min(poll_errors + 1, 10))
                poll_errors += 1
                continue
            except Exception as exc:
                self.logger.error('poll error: %s', exc)
                self.stop.wait(self.config.poll_interval * (min(poll_errors, 5) + 1))
                poll_errors += 1
                continue

            if new_offset and new_offset != offset:
                self.write_text(self.config.offset_file, new_offset)
                offset = new_offset

            for update in updates:
                msg = update.get('message') or update.get('edited_message') or {}
                uid = str(msg.get('chat', {}).get('id', ''))
                if chat_filter and uid != chat_filter:
                    continue
                target = uid if uid else chat_filter
                text = (msg.get('text') or msg.get('caption') or '').strip()
                if not text:
                    text = '[non-text]'
                self.append_log(f'{time.strftime("%Y-%m-%d %H:%M:%S")} USER@{target}: {text}')

                lower = text.lower().strip()
                if lower in ('/start', '/help'):
                    self.send_message(target, self.help_text())
                elif text.startswith('/'):
                    self.send_message(target, f'Unknown: {text}')
                else:
                    try:
                        self._poem_queue.put((target, text), timeout=1)
                        self.send_message(target, f'Working on "{text}"...')
                    except queue.Full:
                        self.send_message(target, 'Too busy, try again later')

            self.stop.wait(self.config.poll_interval)

        return 0


if __name__ == '__main__':
    sys.exit(PoetBot(CONFIG).run())
