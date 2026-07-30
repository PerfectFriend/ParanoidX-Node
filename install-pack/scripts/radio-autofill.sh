#!/bin/bash
# Phase III-I1: Radio auto-fill — downloads free music and content for stations.
# Sources: Internet Archive (public domain), Freesound, own generated content.
# Usage: ./radio-autofill.sh [station_id] [count]

set -e
RADIO_DIR="${HOME}/.local/share/simplex-node/radio"
mkdir -p "$RADIO_DIR"
STATION="${1:-}"
COUNT="${2:-5}"
LOG="${RADIO_DIR}/autofill.log"

log() { echo "[$(date '+%H:%M:%S')] $*" | tee -a "$LOG"; }

download_archive() {
  local query="$1" outdir="$2" max="$3"
  log "Searching Internet Archive for: $query"
  # Use archive.org advanced search JSON API
  local url="https://archive.org/advancedsearch.php?q=${query// /+}+AND+mediatype:(audio)&fl[]=identifier,title,downloads&sort[]=downloads+desc&rows=$max&output=json"
  local data
  data=$(curl -s --max-time 15 "$url" 2>/dev/null || echo '{"response":{"docs":[]}}')
  local ids
  ids=$(echo "$data" | python3 -c "
import json,sys
d=json.load(sys.stdin)
for doc in d.get('response',{}).get('docs',[]):
    print(doc.get('identifier',''))
" 2>/dev/null)
  local count=0
  for id in $ids; do
    [ "$count" -ge "$max" ] && break
    local file_url="https://archive.org/download/${id}/"
    # Try to get first mp3/ogg from the item
    local files_xml
    files_xml=$(curl -s --max-time 10 "${file_url}?output=json" 2>/dev/null || echo '{}')
    local audio_file
    audio_file=$(echo "$files_xml" | python3 -c "
import json,sys
d=json.load(sys.stdin)
files=d.get('files',{})
for name,info in sorted(files.items()):
    if name.endswith(('.mp3','.ogg','.wav')) and 'Modulator' not in str(info):
        print(name)
        break
" 2>/dev/null)
    if [ -n "$audio_file" ]; then
      local dest="${outdir}/archive-${id}-${audio_file}"
      if [ ! -f "$dest" ]; then
        log "Downloading: $audio_file (from $id)"
        curl -sL --max-time 60 "${file_url}${audio_file}" -o "$dest" 2>/dev/null && log "  OK: $(du -h "$dest" | cut -f1)" || log "  FAIL: $audio_file"
        count=$((count + 1))
      fi
    fi
  done
  log "Downloaded $count tracks from Archive.org for '$query'"
}

generate_ai_content() {
  local station="$1" outdir="$2" count="$3"
  local api_url="${API_BASE:-http://localhost:8080}/api/ai/chat"
  for i in $(seq 1 "$count"); do
    local text
    case "$station" in
      steward-ai)
        text="Generate a short market commentary about silver prices and economic outlook, 2-3 sentences."
        ;;
      liberty-voice-en)
        text="Generate a short news bulletin about digital sovereignty and freedom technology, 2-3 sentences."
        ;;
      liberty-voice-ru)
        text="Сгенерируй короткую новостную сводку о цифровом суверенитете, 2-3 предложения."
        ;;
      liberty-voice-es)
        text="Genera un breve boletín de noticias sobre soberanía digital, 2-3 oraciones."
        ;;
      torquemada-monitor)
        text="Generate a system status report for a sovereign network node, mention uptime and security."
        ;;
      *)
        text="Generate a short statement about freedom and sovereignty, 2-3 sentences."
        ;;
    esac
    local resp
    resp=$(curl -s --max-time 30 -X POST "$api_url" \
      -H "Content-Type: application/json" \
      -d "{\"text\":\"$text\"}" 2>/dev/null || echo '{"response":""}')
    local content
    content=$(echo "$resp" | python3 -c "import json,sys;print(json.load(sys.stdin).get('response','') or json.load(sys.stdin).get('reply','') or '')" 2>/dev/null)
    if [ -n "$content" ] && [ "${#content}" -gt 20 ]; then
      local tts_file="${outdir}/ai-${station}-$(date +%s)-${i}.txt"
      echo "$content" > "$tts_file"
      log "AI content for $station: ${content:0:60}..."
    fi
    sleep 2
  done
}

log "=== Radio Auto-Fill Started ==="
log "Station: ${STATION:-all} | Count: $COUNT"

if [ -n "$STATION" ]; then
  STATION_DIR="$RADIO_DIR/stations/$STATION"
  mkdir -p "$STATION_DIR" "$RADIO_DIR"
  case "$STATION" in
    liberty-voice-en|liberty-voice-es|liberty-voice-ru)
      download_archive "public domain music" "$RADIO_DIR" "$COUNT"
      download_archive "creative commons audio" "$RADIO_DIR" "$((COUNT / 2 + 1))"
      generate_ai_content "$STATION" "$STATION_DIR" 2
      ;;
    steward-ai)
      generate_ai_content "$STATION" "$STATION_DIR" "$COUNT"
      ;;
    torquemada-monitor)
      generate_ai_content "$STATION" "$STATION_DIR" "$COUNT"
      download_archive "podcast" "$RADIO_DIR" 2
      ;;
    *)
      download_archive "free music" "$RADIO_DIR" "$COUNT"
      ;;
  esac
else
  # Fill all stations
  for st in liberty-voice-en liberty-voice-es liberty-voice-ru steward-ai torquemada-monitor; do
    log "--- Filling station: $st ---"
    mkdir -p "$RADIO_DIR/stations/$st"
    case "$st" in
      liberty-voice-*) download_archive "public domain music" "$RADIO_DIR" "$((COUNT / 2 + 1))" ;;
      steward-ai)      generate_ai_content "$st" "$RADIO_DIR/stations/$st" 3 ;;
      torquemada-monitor) generate_ai_content "$st" "$RADIO_DIR/stations/$st" 3 ;;
    esac
  done
fi

# Trigger station content sync (SymlinkStationContent)
log "Triggering station content sync..."
curl -s --max-time 5 -X POST "http://localhost:8080/api/radio?action=sync" 2>/dev/null || true

log "=== Radio Auto-Fill Complete ==="
log "Total files in radio dir: $(find "$RADIO_DIR" -maxdepth 1 -type f -name '*.mp3' -o -name '*.ogg' -o -name '*.wav' | wc -l)"
