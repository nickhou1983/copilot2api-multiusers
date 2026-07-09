#!/usr/bin/env bash
#
# test_opus48_image_url.sh — Probe whether the GitHub Copilot upstream model
# Opus 4.8 supports URL image sources (Anthropic "image" block with
# source.type = "url", a.k.a. "image_url").
#
# It calls the LIVE Copilot upstream /v1/messages endpoint directly with curl:
#   1. reads the stored github_token,
#   2. exchanges it for a short-lived copilot token (curl),
#   3. derives the upstream host from the token's proxy-ep,
#   4. sends ONE image-URL request and ONE base64 control request,
#   5. prints the constructed request + verdict.
#
# The github_token and copilot token are NEVER printed. Only the request body
# and the upstream status/response are shown.
#
# Usage:
#   scripts/test_opus48_image_url.sh [account]
# Env overrides:
#   MODEL      (default: claude-opus-4.8)
#   IMAGE_URL  (default: a small public PNG)
#   ACCOUNT    (default: nick.hou1983@outlook.com)
#
set -euo pipefail

ACCOUNT="${1:-${ACCOUNT:-nick.hou1983@outlook.com}}"
MODEL="${MODEL:-claude-opus-4.8}"
IMAGE_URL="${IMAGE_URL:-https://upload.wikimedia.org/wikipedia/commons/thumb/6/6a/PNG_Test.png/320px-PNG_Test.png}"

CRED="$HOME/.config/copilot2api/$ACCOUNT/credentials.json"
UA="GitHubCopilotChat/0.39.0"

# Positive control: a 200x200 solid-color PNG (upstream vision parser needs
# >=200x200). Generated at runtime, base64-encoded, so vision is proven to work.
PNG_B64="$(python3 - <<'PY'
import struct, zlib, binascii, base64
w = h = 200
def chunk(tag, payload):
    body = tag + payload
    return struct.pack(">I", len(payload)) + body + struct.pack(">I", binascii.crc32(body) & 0xFFFFFFFF)
ihdr = struct.pack(">IIBBBBB", w, h, 8, 2, 0, 0, 0)
row = bytes((177, 31, 75)) * w
raw = b"".join(b"\x00" + row for _ in range(h))
png = b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", ihdr) + chunk(b"IDAT", zlib.compress(raw, 9)) + chunk(b"IEND", b"")
print(base64.b64encode(png).decode())
PY
)"

# --- helpers ---------------------------------------------------------------
die() { echo "ERROR: $*" >&2; exit 1; }

command -v curl   >/dev/null || die "curl not found"
command -v python3 >/dev/null || die "python3 not found"
[ -f "$CRED" ] || die "credentials not found: $CRED"

# --- 1. read github_token (never printed) ----------------------------------
GH_TOKEN="$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["github_token"])' "$CRED")"
[ -n "$GH_TOKEN" ] || die "github_token missing in $CRED"

# --- 2. exchange for a copilot token ---------------------------------------
COPILOT_TOKEN="$(curl -sS \
  -H "Authorization: token $GH_TOKEN" \
  -H "User-Agent: $UA" \
  https://api.github.com/copilot_internal/v2/token \
  | python3 -c 'import json,sys;print(json.load(sys.stdin).get("token",""))')"
[ -n "$COPILOT_TOKEN" ] || die "copilot token exchange failed"

# --- 3. derive upstream base URL from proxy-ep -----------------------------
BASE_URL="$(python3 - "$COPILOT_TOKEN" <<'PY'
import sys
tok = sys.argv[1]
base = "https://api.individual.githubcopilot.com"
for part in tok.split(";"):
    if part.startswith("proxy-ep="):
        ep = part[len("proxy-ep="):]
        base = "https://api." + ep[len("proxy."):] if ep.startswith("proxy.") else "https://" + ep
print(base)
PY
)"

echo "=================================================================="
echo " Copilot upstream  : $BASE_URL/v1/messages"
echo " Model             : $MODEL"
echo " Account           : $ACCOUNT"
echo " Image URL         : $IMAGE_URL"
echo "=================================================================="

# --- build request bodies --------------------------------------------------
URL_BODY="$(python3 - "$MODEL" "$IMAGE_URL" <<'PY'
import json,sys
model, url = sys.argv[1], sys.argv[2]
print(json.dumps({
  "model": model,
  "max_tokens": 64,
  "messages": [{
    "role": "user",
    "content": [
      {"type": "image", "source": {"type": "url", "url": url}},
      {"type": "text", "text": "What is in this image? One short sentence."}
    ]
  }]
}, indent=2))
PY
)"

B64_BODY="$(python3 - "$MODEL" "$PNG_B64" <<'PY'
import json,sys
model, data = sys.argv[1], sys.argv[2]
print(json.dumps({
  "model": model,
  "max_tokens": 64,
  "messages": [{
    "role": "user",
    "content": [
      {"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": data}},
      {"type": "text", "text": "Reply with the single word OK."}
    ]
  }]
}, indent=2))
PY
)"

# --- request runner --------------------------------------------------------
run_case() {
  local label="$1" body="$2"
  echo
  echo "################  $label  ################"
  echo "--- Constructed request (curl) -----------------------------------"
  cat <<EOF
curl -sS -X POST "$BASE_URL/v1/messages" \\
  -H "Authorization: Bearer \$COPILOT_TOKEN"   # <-- redacted \\
  -H "User-Agent: $UA" \\
  -H "Editor-Version: vscode/1.111.0" \\
  -H "Editor-Plugin-Version: copilot-chat/0.39.0" \\
  -H "Copilot-Integration-Id: vscode-chat" \\
  -H "Openai-Intent: conversation-agent" \\
  -H "X-Github-Api-Version: 2026-06-01" \\
  -H "Anthropic-Version: 2023-06-01" \\
  -H "Content-Type: application/json" \\
  -d '<body below>'
EOF
  echo "--- Request body -------------------------------------------------"
  echo "$body"
  echo "--- Upstream response --------------------------------------------"
  local resp status http_body
  resp="$(curl -sS -w $'\n__HTTP_STATUS__%{http_code}' -X POST "$BASE_URL/v1/messages" \
    -H "Authorization: Bearer $COPILOT_TOKEN" \
    -H "User-Agent: $UA" \
    -H "Editor-Version: vscode/1.111.0" \
    -H "Editor-Plugin-Version: copilot-chat/0.39.0" \
    -H "Copilot-Integration-Id: vscode-chat" \
    -H "Openai-Intent: conversation-agent" \
    -H "X-Github-Api-Version: 2026-06-01" \
    -H "Anthropic-Version: 2023-06-01" \
    -H "Content-Type: application/json" \
    -d "$body" || true)"
  status="${resp##*__HTTP_STATUS__}"
  http_body="${resp%$'\n'__HTTP_STATUS__*}"
  echo "HTTP $status"
  echo "$http_body" | python3 -c 'import json,sys;d=sys.stdin.read()
try:
    print(json.dumps(json.loads(d), indent=2, ensure_ascii=False)[:1500])
except Exception:
    print(d[:1500])'
  echo "$status" > "$STATUS_FILE"
}

STATUS_FILE="$(mktemp)"
trap 'rm -f "$STATUS_FILE"' EXIT

run_case "CASE 1: image via URL (image_url)  [source.type=url]" "$URL_BODY"
URL_STATUS="$(cat "$STATUS_FILE")"
run_case "CASE 2: image via base64 (control)  [source.type=base64]" "$B64_BODY"
B64_STATUS="$(cat "$STATUS_FILE")"

# --- verdict ---------------------------------------------------------------
echo
echo "=================================================================="
echo " VERDICT for $MODEL (Copilot upstream)"
echo "------------------------------------------------------------------"
echo " image via URL     (image_url)  -> HTTP $URL_STATUS"
echo " image via base64  (control)    -> HTTP $B64_STATUS"
echo "------------------------------------------------------------------"
if [ "$URL_STATUS" = "200" ]; then
  echo " RESULT: SUPPORTED — upstream accepted the URL image source."
elif [[ "$URL_STATUS" =~ ^4 ]]; then
  if [ "$B64_STATUS" = "200" ]; then
    echo " RESULT: NOT SUPPORTED — URL image source rejected ($URL_STATUS),"
    echo "         while base64 vision works (200). image_url is unsupported."
  else
    echo " RESULT: NOT SUPPORTED — URL image source rejected ($URL_STATUS);"
    echo "         base64 control also failed ($B64_STATUS), check auth/model."
  fi
else
  echo " RESULT: INCONCLUSIVE — unexpected status $URL_STATUS."
fi
echo "=================================================================="
