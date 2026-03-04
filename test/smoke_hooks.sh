#!/usr/bin/env bash
#
# Smoke tests for the plugin hook system.
#
# Builds wk + recipe-lint into a temp directory, sets up an isolated WK_HOME,
# and validates push-related hook behavior without touching real install paths.
#
# Usage:  ./test/smoke_hooks.sh
# Exit:   0 on success, 1 on first failure

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORKDIR="$(mktemp -d)"

cleanup() { rm -rf "$WORKDIR"; }
trap cleanup EXIT

PASS=0
FAIL=0

pass() { PASS=$((PASS + 1)); echo "  PASS: $1"; }
fail() { FAIL=$((FAIL + 1)); echo "  FAIL: $1"; }

# ---------- Build artifacts into workdir ----------

echo "==> Building wk binary..."
go build -o "$WORKDIR/bin/wk" "$REPO_ROOT/cmd/wk"

echo "==> Building recipe-lint plugin..."
(cd "$REPO_ROOT/plugins/recipe-lint" && go build -o "$WORKDIR/bin/recipe-lint" .)

WK="$WORKDIR/bin/wk"

# ---------- Set up isolated environment ----------

export WK_HOME="$WORKDIR/wk-home"
export HOME="$WORKDIR/fake-home"   # prevent auth/keyring from touching real home
mkdir -p "$WK_HOME/plugins" "$HOME"

# ---------- Helper: scaffold a minimal wk project ----------

PROJECT="$WORKDIR/project"
mkdir -p "$PROJECT/recipes"

cat > "$PROJECT/wk.toml" <<'TOML'
workspace = "test"

[[sync]]
server_path = "All projects"
local_path  = "recipes"
TOML

# ---------- Test 1: push without plugins shows no hook warnings ----------

echo ""
echo "==> Test 1: push with no plugins installed (expect no hook output)"
cd "$PROJECT"

# push will fail because there's no API client, but it should get past hooks first.
# We check stderr does NOT contain "hook" or "Warning".
OUTPUT=$("$WK" push 2>&1 || true)
if echo "$OUTPUT" | grep -qi "hook"; then
  fail "push with no plugins should not mention hooks"
else
  pass "push with no plugins produces no hook output"
fi

# ---------- Test 2: push with passthrough plugin ----------

echo ""
echo "==> Test 2: install recipe-lint, push should pass hooks"

# Install the plugin scaffold into WK_HOME
PLUGIN_DEST="$WK_HOME/plugins/recipe-lint"
mkdir -p "$PLUGIN_DEST"
cp "$WORKDIR/bin/recipe-lint" "$PLUGIN_DEST/"
cp "$REPO_ROOT/plugins/recipe-lint/plugin.toml" "$PLUGIN_DEST/"

cd "$PROJECT"
OUTPUT=$("$WK" push 2>&1 || true)
if echo "$OUTPUT" | grep -qi "hook.*failed\|hook.*blocked"; then
  fail "passthrough plugin should not block push"
else
  pass "passthrough plugin allows push to proceed"
fi

# ---------- Test 3: --skip-hooks bypasses hook entirely ----------

echo ""
echo "==> Test 3: --skip-hooks flag"
cd "$PROJECT"
OUTPUT=$("$WK" push --skip-hooks 2>&1 || true)
if echo "$OUTPUT" | grep -qi "hook"; then
  fail "--skip-hooks should suppress all hook output"
else
  pass "--skip-hooks bypasses hooks"
fi

# ---------- Test 4: --dry-run does not invoke hooks ----------

echo ""
echo "==> Test 4: --dry-run does not invoke hooks"
cd "$PROJECT"
OUTPUT=$("$WK" push --dry-run 2>&1 || true)
if echo "$OUTPUT" | grep -qi "hook"; then
  fail "--dry-run should not invoke hooks"
else
  pass "--dry-run skips hooks"
fi

# ---------- Test 5: failing plugin blocks push ----------

echo ""
echo "==> Test 5: failing plugin blocks push"

# Replace the passthrough binary with one that returns passed=false.
cat > "$WORKDIR/fail_plugin.go" <<'GO'
package main

import (
	"bufio"
	"encoding/json"
	"os"
)

type req struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      int             `json:"id"`
}
type resp struct {
	JSONRPC string `json:"jsonrpc"`
	Result  any    `json:"result,omitempty"`
	ID      int    `json:"id"`
}

func main() {
	s := bufio.NewScanner(os.Stdin)
	e := json.NewEncoder(os.Stdout)
	for s.Scan() {
		var r req
		json.Unmarshal(s.Bytes(), &r)
		switch r.Method {
		case "shutdown":
			e.Encode(resp{JSONRPC: "2.0", Result: "ok", ID: r.ID})
			return
		case "lint.pre_push":
			e.Encode(resp{JSONRPC: "2.0", Result: map[string]any{
				"passed": false,
				"diagnostics": []map[string]string{
					{"file": "recipes/bad.json", "severity": "error", "message": "invalid recipe", "rule": "schema"},
				},
			}, ID: r.ID})
		default:
			e.Encode(resp{JSONRPC: "2.0", Result: "ok", ID: r.ID})
		}
	}
}
GO

go build -o "$PLUGIN_DEST/recipe-lint" "$WORKDIR/fail_plugin.go"

cd "$PROJECT"
OUTPUT=$("$WK" push 2>&1 || true)

if echo "$OUTPUT" | grep -q "push blocked"; then
  pass "failing plugin blocks push"
else
  fail "failing plugin should block push, got: $OUTPUT"
fi

if echo "$OUTPUT" | grep -q "invalid recipe"; then
  pass "diagnostic message is printed"
else
  fail "diagnostic message should appear in output, got: $OUTPUT"
fi

# ---------- Test 6: --skip-hooks bypasses even a failing plugin ----------

echo ""
echo "==> Test 6: --skip-hooks bypasses failing plugin"
cd "$PROJECT"
OUTPUT=$("$WK" push --skip-hooks 2>&1 || true)
if echo "$OUTPUT" | grep -qi "blocked"; then
  fail "--skip-hooks should bypass failing plugin"
else
  pass "--skip-hooks bypasses failing plugin"
fi

# ---------- Summary ----------

echo ""
echo "================================"
echo "  Results: $PASS passed, $FAIL failed"
echo "================================"

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
