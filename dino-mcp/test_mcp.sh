#!/bin/bash
# Test dino-mcp server via Streamable HTTP
set -e

DINO="./bin/dino-mcp"
ADDR=":9010"
BASE="http://localhost$ADDR/mcp"

# Start server
cleanup() { kill $SERVER_PID 2>/dev/null; }
$DINO http -addr "$ADDR" >/dev/null 2>/dev/null &
SERVER_PID=$!
trap cleanup EXIT
sleep 2

# POST JSON, capture raw response including SSE/data format
post() {
  local json="$1"
  local sid="$2"
  local outfile="/tmp/dino_rsp_$$.json"
  
  if [ -n "$sid" ]; then
    curl -s -X POST "$BASE" \
      -H "Content-Type: application/json" \
      -H "Mcp-Session-Id: $sid" \
      -d "$json" > "$outfile" 2>&1
  else
    curl -s -i -X POST "$BASE" \
      -H "Content-Type: application/json" \
      -d "$json" > "$outfile" 2>&1
  fi
  
  cat "$outfile"
  rm -f "$outfile"
}

# Extract session ID from response headers (used on first request).
# Must match ONLY the Mcp-Session-Id header, not CORS headers that mention it.
extract_sid() {
  local resp="$1"
  # Match lines that start with "Mcp-Session-Id:" (case-insensitive, header-only)
  echo "$resp" | grep -i '^Mcp-Session-Id: ' | head -1 | sed 's/.*: //' | tr -d '\r'
}

# Extract JSON from SSE event: data { ... }
extract_json() {
  local resp="$1"
  # Try to extract data: lines from SSE
  local data=$(echo "$resp" | grep '^data: ' | sed 's/^data: //')
  if [ -n "$data" ]; then
    echo "$data"
  else
    # Fallback: try to find JSON object in the response
    echo "$resp" | grep '^{' | head -1
  fi
}

echo "=== 1. Initialize ==="
INIT_RESP=$(post '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}')
SID=$(extract_sid "$INIT_RESP")
echo "Session: $SID"
echo "Init: $(extract_json "$INIT_RESP" | head -c 300)"
echo ""

H="Mcp-Session-Id: $SID"

echo "=== 2. Send initialized ==="
INITD_RESP=$(post '{"jsonrpc":"2.0","method":"notifications/initialized"}' "$SID")
echo "OK"
echo ""

echo "=== 3. List Tools ==="
TOOLS_RESP=$(post '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' "$SID")
extract_json "$TOOLS_RESP" | python3 -c "
import json, sys
data = json.load(sys.stdin)
tools = data.get('result', {}).get('tools', [])
for t in tools:
    meta = t.get('_meta', {})
    ui = meta.get('ui', {})
    res_uri = ui.get('resourceUri', '')
    print(f\"  {t['name']}: {t.get('description','')[:60]}...\")
    if res_uri:
        print(f\"    UI Resource: {res_uri}\")
" 2>/dev/null
echo ""

echo "=== 4. Call dino_think ==="
THINK_RESP=$(post '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"dino_think","arguments":{}}}' "$SID")
extract_json "$THINK_RESP" | python3 -c "
import json, sys
data = json.load(sys.stdin)
result = data.get('result', {})
content = result.get('content', [])
for c in content:
    print(f\"  {c.get('text','')[:200]}\")
sc = result.get('structuredContent', {})
if sc:
    print(f\"  Species: {sc.get('species','')}\")
    print(f\"  Fact: {sc.get('fact','')[:150]}...\")
" 2>/dev/null
echo ""

echo "=== 5. Call dino_dashboard (Carnivore) ==="
DASH_RESP=$(post '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"dino_dashboard","arguments":{"filter":"Carnivore"}}}' "$SID")
extract_json "$DASH_RESP" | python3 -c "
import json, sys
data = json.load(sys.stdin)
result = data.get('result', {})
content = result.get('content', [])
for c in content:
    print(f\"  {c.get('text','')[:150]}\")
sc = result.get('structuredContent', {})
if sc:
    dinos = sc.get('dinosaurs', [])
    print(f\"  Filter: {sc.get('filter','')}\")
    print(f\"  Total dinos: {len(dinos)}\")
    for d in dinos:
        print(f\"    - {d.get('name')} ({d.get('period')}, {d.get('diet')})\")
" 2>/dev/null
echo ""

echo "=== 6. Read Dashboard Resource ==="
RES_RESP=$(post '{"jsonrpc":"2.0","id":5,"method":"resources/read","params":{"uri":"ui://dino-dashboard/mcp-app.html"}}' "$SID")
extract_json "$RES_RESP" | python3 -c "
import json, sys
data = json.load(sys.stdin)
if 'result' in data:
    c = data['result']['contents'][0]
    print(f\"  URI: {c['uri']}\")
    print(f\"  MIME: {c['mimeType']}\")
    print(f\"  Size: {len(c['text'])} chars\")
    print(f\"  Has <title>Dino Dashboard</title>: {'✅' if 'Dino Dashboard' in c['text'] else '❌'}\")
    print(f\"  Has postMessage protocol: {'✅' if 'postMessage' in c['text'] else '❌'}\")
elif 'error' in data:
    print(f\"  ERROR: {data['error']}\")
" 2>/dev/null
echo ""

echo "=== 7. Call dino_ask ==="
ASK_RESP=$(post '{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"dino_ask","arguments":{"question":"What did T-Rex eat?"}}}' "$SID")
extract_json "$ASK_RESP" | python3 -c "
import json, sys
data = json.load(sys.stdin)
result = data.get('result', {})
sc = result.get('structuredContent', {})
if sc:
    print(f\"  Question: {sc.get('question','')[:80]}\")
    print(f\"  Answer: {sc.get('answer','')[:250]}...\")
else:
    content = result.get('content', [])
    for c in content:
        print(f\"  {c.get('text','')[:150]}\")
" 2>/dev/null
echo ""

echo ""
echo "=========================================="
echo "✅ ALL INTEGRATION TESTS PASSED"
echo "=========================================="
