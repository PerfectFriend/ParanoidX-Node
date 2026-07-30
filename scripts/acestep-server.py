#!/usr/bin/env python3
"""
acestep-server — local AI radio audio generator for simplex-node.

Replaces the separate Acestep laptop (192.168.1.129:8001) with a local
HTTP server that generates speech audio via edge-tts (Microsoft Edge TTS).

Endpoints (same as Acestep remote):
  GET  /health            → {"status":"ok"}
  POST /generate          → {"audio_url":"/cache/...mp3","duration":N,"track_id":"...","created_at":"..."}

Styles (mapped to voices):
  gospel  → black preacher (US Male)
  tragedy → dramatic news (US Male)
  royal   → majestic decree (RU Male)
  music   → ambient description (not implemented — returns placeholder)

Usage:
  pip install edge-tts fastapi uvicorn
  python acestep-server.py --port 8001 --cache ~/.cache/acestep
"""

import argparse
import hashlib
import json
import os
import subprocess
import sys
import time
import uuid
from datetime import datetime, timezone
from pathlib import Path

CACHE_DIR = os.path.expanduser("~/.cache/acestep")

STYLE_VOICES = {
    "gospel":  "en-US-ChristopherNeural",
    "tragedy": "en-US-GuyNeural",
    "royal":   "ru-RU-DmitryNeural",
    "music":   "en-US-JennyNeural",
}

STYLE_PROMPTS = {
    "gospel":  " — проповедь спасения через silver, радостно, энергично",
    "tragedy": " — BREAKING NEWS, трагедия в голосе, драматично, срочно",
    "royal":   " — королевский указ, величественно, торжественно, голос Короля",
    "music":   " — атмосферная музыка острова, расслабляющая",
}


def generate_audio(text, voice, cache_key):
    """Generate speech via edge-tts, save to cache, return local path."""
    os.makedirs(CACHE_DIR, exist_ok=True)
    output_path = os.path.join(CACHE_DIR, f"{cache_key}.mp3")
    if os.path.exists(output_path):
        return output_path

    cmd = ["edge-tts", "--voice", voice, "--text", text, "--write-media", output_path]
    try:
        subprocess.run(cmd, check=True, capture_output=True, timeout=60)
    except subprocess.CalledProcessError as e:
        raise RuntimeError(f"edge-tts failed: {e.stderr.decode()}")
    except FileNotFoundError:
        raise RuntimeError("edge-tts not installed. Run: pip install edge-tts")

    if not os.path.exists(output_path):
        raise RuntimeError("edge-tts produced no output")

    return output_path


def get_audio_duration(path):
    """Get MP3 duration via ffprobe."""
    try:
        r = subprocess.run(
            ["ffprobe", "-v", "quiet", "-print_format", "json", "-show_format", path],
            capture_output=True, text=True, timeout=10,
        )
        data = json.loads(r.stdout)
        return float(data["format"]["duration"])
    except Exception:
        return 0


# ---- FastAPI server ----
try:
    from fastapi import FastAPI, HTTPException
    from fastapi.responses import FileResponse, JSONResponse
    from pydantic import BaseModel
    import uvicorn
except ImportError:
    print("ERROR: Install dependencies: pip install fastapi uvicorn pydantic")
    sys.exit(1)

app = FastAPI(title="acestep-server", version="1.0.0")


class GenerateRequest(BaseModel):
    prompt: str = ""
    style: str = "gospel"
    duration: int = 30
    voice: str = ""


class GenerateResponse(BaseModel):
    audio_url: str
    duration: int
    track_id: str
    created_at: str


@app.get("/health")
async def health():
    return {"status": "ok", "service": "acestep-local", "version": "1.0.0"}


@app.post("/generate")
async def generate(req: GenerateRequest):
    style = req.style or "gospel"
    if style not in STYLE_VOICES:
        raise HTTPException(400, f"Unknown style: {style}. Choose: {list(STYLE_VOICES.keys())}")

    voice = req.voice or STYLE_VOICES[style]
    text = req.prompt
    if not text:
        text = f"{style.capitalize()} broadcast from Saint Mary Liberty Island{STYLE_PROMPTS.get(style, '')}"
    else:
        text = f"{text}{STYLE_PROMPTS.get(style, '')}"

    cache_key = hashlib.sha256(text.encode()).hexdigest()[:16]
    track_id = f"acestep-{style}-{cache_key}"

    try:
        audio_path = generate_audio(text, voice, cache_key)
    except RuntimeError as e:
        raise HTTPException(500, str(e))

    duration = get_audio_duration(audio_path)
    if duration < 1:
        duration = req.duration

    return {
        "audio_url": f"/cache/{cache_key}.mp3",
        "duration": int(duration),
        "track_id": track_id,
        "created_at": datetime.now(timezone.utc).isoformat(),
    }


@app.get("/cache/{filename}")
async def serve_cache(filename: str):
    path = os.path.join(CACHE_DIR, filename)
    if not os.path.exists(path):
        raise HTTPException(404, "File not found")
    return FileResponse(path, media_type="audio/mpeg")


def main():
    parser = argparse.ArgumentParser(description="Acestep local AI radio generator")
    parser.add_argument("--port", type=int, default=8001, help="Listen port (default: 8001)")
    parser.add_argument("--host", default="127.0.0.1", help="Bind address (default: 127.0.0.1)")
    parser.add_argument("--cache", default=CACHE_DIR, help=f"Cache directory (default: {CACHE_DIR})")
    args = parser.parse_args()

    global CACHE_DIR
    CACHE_DIR = args.cache
    os.makedirs(CACHE_DIR, exist_ok=True)

    print(f"🎙 acestep-server starting on http://{args.host}:{args.port}")
    print(f"   Cache: {CACHE_DIR}")
    print(f"   Styles: {list(STYLE_VOICES.keys())}")
    print(f"   Voices: {list(STYLE_VOICES.values())}")
    print()
    print("   curl http://127.0.0.1:{args.port}/health")
    print('   curl -X POST http://127.0.0.1:{args.port}/generate \\')
    print('     -H "Content-Type: application/json" \\')
    print('     -d \'{"prompt":"Серебро течёт в жилах Острова","style":"royal","duration":20}\'')
    print()

    uvicorn.run(app, host=args.host, port=args.port, log_level="info")


if __name__ == "__main__":
    main()
