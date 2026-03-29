#!/usr/bin/env bash
# Integration test for ws daemon — tests the REAL spawn flow.
#
# This test starts the daemon from /tmp (NOT a git repo) to prove the
# daemon works regardless of its working directory. It then creates a
# real root workstream, spawns a child via the daemon socket, and
# verifies that panes actually appear.
set -euo pipefail

WS="${WS:-$HOME/bin/ws}"
TMPDIR_REAL="${TMPDIR:-/tmp/}"
SOCKET="${TMPDIR_REAL}ws-relay.sock"
STATE="$HOME/.config/ws/state.json"
LOG="${TMPDIR_REAL}ws-daemon.log"
REPO="/Users/kencrimmins/Documents/GitHub/Grimoire"
PASS=0
FAIL=0

pass() { echo "  ✓ $1"; PASS=$((PASS + 1)); }
fail() { echo "  ✗ $1"; FAIL=$((FAIL + 1)); }

cleanup() {
    echo "--- cleanup ---"
    $WS daemon stop 2>/dev/null || true
    tmux kill-session -t "ws/itest" 2>/dev/null || true
    # Remove test worktrees
    cd "$REPO"
    git worktree remove --force ../itest 2>/dev/null || true
    git worktree remove --force ../itest-child1 2>/dev/null || true
    git branch -D ws-itest 2>/dev/null || true
    git branch -D ws-itest-child1 2>/dev/null || true
    # Clean state file
    python3 -c "
import json, os
path = os.path.expanduser('~/.config/ws/state.json')
try:
    with open(path) as f: data = json.load(f)
    for k in list(data.get('nodes',{}).keys()):
        if k.startswith('itest'):
            del data['nodes'][k]
    with open(path, 'w') as f: json.dump(data, f, indent=2)
except: pass
" 2>/dev/null || true
    rm -f "$LOG"
}

trap cleanup EXIT

echo "=== ws relay integration test (real spawn) ==="
echo ""

# -------------------------------------------------------------------
# 0. Clean slate
# -------------------------------------------------------------------
echo "--- setup ---"
cleanup 2>/dev/null
echo ""

# -------------------------------------------------------------------
# 1. Start daemon from /tmp — NOT a git repo
# -------------------------------------------------------------------
echo "--- test: daemon starts from non-repo directory ---"
(cd /tmp && $WS daemon start 2>&1) || true
sleep 1

if [ -S "$SOCKET" ]; then
    pass "daemon socket exists"
else
    fail "daemon socket missing"
    echo "FATAL: cannot continue without daemon"
    exit 1
fi

if [ -f "${TMPDIR_REAL}ws-relay.pid" ]; then
    pass "pid file written"
else
    fail "pid file missing"
fi

if [ -f "$LOG" ] && [ -s "$LOG" ]; then
    pass "daemon log file has content"
else
    fail "daemon log file empty or missing"
fi
echo ""

# -------------------------------------------------------------------
# 2. Create a real root workstream via `ws add` (from inside the repo)
# -------------------------------------------------------------------
echo "--- test: create root workstream ---"
(cd "$REPO" && $WS add itest --agent amp --task "" 2>&1) || true
sleep 1

# Verify tmux session exists
if tmux has-session -t "ws/itest" 2>/dev/null; then
    pass "tmux session 'ws/itest' created"
else
    fail "tmux session 'ws/itest' not found"
fi

# Verify git worktree exists
if [ -d "$REPO/../itest" ]; then
    pass "git worktree created at ../itest"
else
    fail "git worktree not found at ../itest"
fi

# Verify state file has the node
if python3 -c "
import json
with open('$STATE') as f: data = json.load(f)
node = data['nodes']['itest']
assert node['work_dir'], 'no work_dir'
assert node['session'] == 'ws/itest', 'bad session'
print('ok')
" 2>/dev/null | grep -q ok; then
    pass "state file has 'itest' node with work_dir and session"
else
    fail "state file missing or malformed 'itest' node"
fi

# Count panes before spawn
PANES_BEFORE=$(tmux list-panes -t "ws/itest" 2>/dev/null | wc -l | tr -d ' ')
echo "  panes before spawn: $PANES_BEFORE"
echo ""

# -------------------------------------------------------------------
# 3. Spawn a child via the daemon socket (simulates relay_spawn MCP call)
# -------------------------------------------------------------------
echo "--- test: spawn child via daemon socket ---"
# Use a python script to send spawn and read the response with a timeout,
# since createWorkstream takes a few seconds (worktree creation).
SPAWN_RESP=$(python3 -c "
import socket, json, time
s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
s.connect('$SOCKET')
s.settimeout(15)
env = {'action':'spawn','payload':{'parent_id':'itest','name':'child1','task':'do something'}}
s.sendall((json.dumps(env) + '\n').encode())
try:
    data = b''
    while True:
        chunk = s.recv(4096)
        if not chunk: break
        data += chunk
        if b'\n' in data: break
    print(data.decode().strip())
except socket.timeout:
    print('TIMEOUT')
s.close()
" 2>&1)
echo "  spawn response: $SPAWN_RESP"

if echo "$SPAWN_RESP" | grep -q '"spawned"'; then
    pass "daemon reported child spawned"
elif echo "$SPAWN_RESP" | grep -q '"agent_id"'; then
    pass "daemon reported child spawned (got agent_id in response)"
else
    fail "spawn failed: $SPAWN_RESP"
fi

# Give the child time to start (worktree creation + tmux split + agent-run)
sleep 3

# Verify a new pane appeared
PANES_AFTER=$(tmux list-panes -t "ws/itest" 2>/dev/null | wc -l | tr -d ' ')
echo "  panes after spawn: $PANES_AFTER"

if [ "$PANES_AFTER" -gt "$PANES_BEFORE" ]; then
    pass "new pane appeared in tmux session"
else
    fail "no new pane appeared (before=$PANES_BEFORE, after=$PANES_AFTER)"
fi

# Verify child worktree was created
if [ -d "$REPO/../itest-child1" ]; then
    pass "child git worktree created at ../itest-child1"
else
    fail "child git worktree not found at ../itest-child1"
fi

# Verify child is in state file
if python3 -c "
import json
with open('$STATE') as f: data = json.load(f)
node = data['nodes']['itest/child1']
assert node['parent_id'] == 'itest', 'bad parent'
print('ok')
" 2>/dev/null | grep -q ok; then
    pass "state file has 'itest/child1' node with correct parent"
else
    fail "state file missing or malformed 'itest/child1' node"
fi
echo ""

# -------------------------------------------------------------------
# 4. Test message delivery to the child pane
# -------------------------------------------------------------------
echo "--- test: message delivery to spawned child ---"

# First, register manually since agent-run may not have connected
# (amp isn't installed in test env). Get the child's pane ID from tmux.
CHILD_PANE=$(tmux list-panes -t "ws/itest" -F "#{pane_id}" 2>/dev/null | tail -1)
echo "  child pane id: $CHILD_PANE"

python3 -c "
import json
env = {'action':'register','payload':{'agent_id':'itest/child1','parent_id':'itest','agent':'test','pane_id':'$CHILD_PANE'}}
print(json.dumps(env))
" | socat - UNIX-CONNECT:"$SOCKET" > /dev/null 2>&1 || true
sleep 0.3

TEST_MSG="SPAWN_MSG_$(date +%s)"
python3 -c "
import json
msg = {'from':'itest','to':'itest/child1','type':'task','content':'$TEST_MSG','time':'2025-01-01T00:00:00Z'}
env = {'action':'send','payload':msg}
print(json.dumps(env))
" | socat - UNIX-CONNECT:"$SOCKET" > /tmp/ws-itest-send.txt 2>&1 || true
sleep 0.5

SEND_RESP=$(cat /tmp/ws-itest-send.txt)
if echo "$SEND_RESP" | grep -q '"delivered"'; then
    pass "daemon reported message delivered to child"
else
    fail "send to child failed: $SEND_RESP"
fi

PANE_CONTENT=$(tmux capture-pane -t "$CHILD_PANE" -p 2>&1)
if echo "$PANE_CONTENT" | grep -q "$TEST_MSG"; then
    pass "message visible in child pane"
else
    fail "message NOT visible in child pane"
fi
echo ""

# -------------------------------------------------------------------
# 4b. Test SHORT NAME resolution (parent sends to "child1" not "itest/child1")
#     This is the actual bug: agents use short names like "Explore", not "auth/Explore"
# -------------------------------------------------------------------
echo "--- test: short name resolution ---"

SHORT_MSG="SHORT_NAME_$(date +%s)"
python3 -c "
import json
msg = {'from':'itest','to':'child1','type':'task','content':'$SHORT_MSG','time':'2025-01-01T00:00:00Z'}
env = {'action':'send','payload':msg}
print(json.dumps(env))
" | socat - UNIX-CONNECT:"$SOCKET" > /tmp/ws-itest-short.txt 2>&1 || true
sleep 0.5

SHORT_RESP=$(cat /tmp/ws-itest-short.txt)
if echo "$SHORT_RESP" | grep -q '"delivered"'; then
    pass "short name 'child1' resolved to 'itest/child1'"
else
    fail "short name resolution failed: $SHORT_RESP"
fi

PANE_CONTENT2=$(tmux capture-pane -t "$CHILD_PANE" -p 2>&1)
if echo "$PANE_CONTENT2" | grep -q "$SHORT_MSG"; then
    pass "short-name message visible in child pane"
else
    fail "short-name message NOT visible in child pane"
fi

if grep -q "resolved short name" "$LOG" 2>/dev/null; then
    pass "daemon log shows short name resolution"
else
    fail "daemon log missing short name resolution"
fi
echo ""

# -------------------------------------------------------------------
# 5. Verify daemon log captured the spawn + delivery
# -------------------------------------------------------------------
echo "--- test: daemon logging ---"
if grep -q "spawn requested.*itest/child1" "$LOG" 2>/dev/null; then
    pass "daemon log recorded spawn request"
else
    fail "daemon log missing spawn record"
fi

if grep -q "delivered message.*itest/child1" "$LOG" 2>/dev/null; then
    pass "daemon log recorded message delivery"
else
    fail "daemon log missing delivery record"
fi
echo ""

# -------------------------------------------------------------------
# 6. Cleanup and verify
# -------------------------------------------------------------------
echo "--- test: cleanup ---"
tmux kill-session -t "ws/itest" 2>/dev/null
if ! tmux has-session -t "ws/itest" 2>/dev/null; then
    pass "tmux session cleaned up"
else
    fail "tmux session still exists"
fi

$WS daemon stop 2>/dev/null
sleep 0.3
if [ ! -S "$SOCKET" ]; then
    pass "daemon socket removed"
else
    fail "daemon socket still exists"
fi
echo ""

# -------------------------------------------------------------------
# Summary
# -------------------------------------------------------------------
echo "=== Results: $PASS passed, $FAIL failed ==="
if [ "$FAIL" -gt 0 ]; then
    echo ""
    echo "--- daemon log tail ---"
    tail -20 "$LOG" 2>/dev/null || echo "(no log)"
    exit 1
fi
