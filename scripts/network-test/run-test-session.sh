#!/usr/bin/env bash
#
# run-test-session.sh — End-to-end TalkGo network test automation.
#
# 1. (Optional) Apply network profile via simulate-4g.sh
# 2. Build and start the TalkGo server in background
# 3. Run loadgen against the server for the specified duration
# 4. Parse server logs for chunk_latency metrics
# 5. Generate consolidated JSON report
# 6. Clean up (kill server, reset network)
#
# Usage:
#   ./run-test-session.sh -Profile wifi-home -Duration 30s
#   ./run-test-session.sh -Duration 10s -SkipSimulation
#   ./run-test-session.sh -h
#
# Flags:
#   -Profile <name>     Network profile (default: wifi-home)
#   -Duration <dur>     Test duration (default: 60s)
#   -Output <path>      Report output path (default: ./report-<timestamp>.json)
#   -SkipSimulation     Skip network simulation (for baseline)
#   -ServerBinary <path> Path to pre-built server binary (default: build it)
#   -LoadgenBinary <path> Path to pre-built loadgen binary (default: build it)
#   -h, --help          Show this help

set -euo pipefail

# ── Defaults ────────────────────────────────────────────────────────────────
PROFILE="wifi-home"
DURATION="60s"
TIMESTAMP="$(date +%Y%m%dT%H%M%S)"
OUTPUT="./report-${TIMESTAMP}.json"
SKIP_SIM=0
SERVER_BIN=""
LOADGEN_BIN=""
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# ── Colors ──────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Colour

info()  { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }

# ── Help ────────────────────────────────────────────────────────────────────
usage() {
    cat <<'HELP'
TalkGo Run Test Session — End-to-end network test automation

USAGE:
  ./run-test-session.sh [flags]

FLAGS:
  -Profile <name>       Network profile (default: wifi-home)
  -Duration <duration>  Test duration (default: 60s)
  -Output <path>        Report output path
  -SkipSimulation       Skip network simulation (baseline test)
  -ServerBinary <path>  Path to pre-built server binary
  -LoadgenBinary <path> Path to pre-built loadgen binary
  -h, --help            Show this help

EXAMPLES:
  # 30-second test with 4G simulation
  ./run-test-session.sh -Profile 4g -Duration 30s

  # Baseline test without any network simulation
  ./run-test-session.sh -Duration 10s -SkipSimulation

  # Use pre-built binaries
  ./run-test-session.sh -ServerBinary ./talkgo-server -LoadgenBinary ./loadgen

REQUIREMENTS:
  - Go 1.23+ (for building from source)
  - jq (for log parsing)
  - sudo (if applying network simulation)
HELP
}

# ── Parse flags ────────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
    case "$1" in
        -Profile)        PROFILE="$2"; shift 2 ;;
        -Duration)       DURATION="$2"; shift 2 ;;
        -Output)         OUTPUT="$2"; shift 2 ;;
        -SkipSimulation) SKIP_SIM=1; shift ;;
        -ServerBinary)   SERVER_BIN="$2"; shift 2 ;;
        -LoadgenBinary)  LOADGEN_BIN="$2"; shift 2 ;;
        -h|--help)       usage; exit 0 ;;
        *)               echo "Unknown flag: $1"; usage; exit 1 ;;
    esac
done

# ── Cleanup handler ─────────────────────────────────────────────────────────
SERVER_PID=""
cleanup() {
    local exit_code=$?
    info "Cleaning up..."

    # Kill server if running.
    if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
        info "Stopping server (PID $SERVER_PID)..."
        kill "$SERVER_PID" 2>/dev/null || true
        wait "$SERVER_PID" 2>/dev/null || true
        ok "Server stopped."
    fi

    # Reset network simulation if it was applied.
    if [[ $SKIP_SIM -eq 0 ]]; then
        info "Resetting network simulation..."
        "${SCRIPT_DIR}/simulate-4g.sh" -Reset 2>/dev/null || true
        ok "Network simulation reset."
    fi

    # Remove temp server log.
    if [[ -n "${SERVER_LOG:-}" && -f "$SERVER_LOG" ]]; then
        rm -f "$SERVER_LOG"
    fi

    if [[ $exit_code -eq 0 ]]; then
        ok "Test session completed."
    else
        error "Test session failed with exit code $exit_code."
    fi
    exit $exit_code
}
trap cleanup EXIT INT TERM

# ── Prerequisites ───────────────────────────────────────────────────────────
if ! command -v go &>/dev/null; then
    error "Go is required. Install Go 1.23+."
    exit 1
fi

if ! command -v jq &>/dev/null; then
    warn "jq not found. Install it for detailed log parsing:"
    warn "  apt-get install jq  (Debian/Ubuntu)"
    warn "  brew install jq    (macOS)"
    warn "Continuing without jq — server logs will not be parsed."
    HAS_JQ=0
else
    HAS_JQ=1
fi

# ── Step 0: Apply network simulation ────────────────────────────────────────
if [[ $SKIP_SIM -eq 0 ]]; then
    info "Applying network profile '${PROFILE}'..."
    if [[ $EUID -ne 0 ]]; then
        warn "Network simulation requires sudo. Trying with sudo..."
        sudo "${SCRIPT_DIR}/simulate-4g.sh" -Profile "$PROFILE"
    else
        "${SCRIPT_DIR}/simulate-4g.sh" -Profile "$PROFILE"
    fi
    ok "Network profile '${PROFILE}' applied."
else
    info "Skipping network simulation (baseline mode)."
fi

# ── Step 1: Build or locate server binary ──────────────────────────────────
if [[ -z "$SERVER_BIN" ]]; then
    SERVER_BIN="/tmp/talkgo-server-${TIMESTAMP}"
    info "Building server binary..."
    cd "$PROJECT_DIR"
    go build -o "$SERVER_BIN" ./cmd/server
    ok "Server binary built: ${SERVER_BIN}"
else
    info "Using pre-built server binary: ${SERVER_BIN}"
fi

# ── Step 2: Build or locate loadgen binary ─────────────────────────────────
if [[ -z "$LOADGEN_BIN" ]]; then
    LOADGEN_BIN="/tmp/loadgen-${TIMESTAMP}"
    info "Building loadgen binary..."
    cd "$PROJECT_DIR"
    go build -o "$LOADGEN_BIN" ./cmd/loadgen
    ok "Loadgen binary built: ${LOADGEN_BIN}"
else
    info "Using pre-built loadgen binary: ${LOADGEN_BIN}"
fi

# ── Step 3: Start server in background ─────────────────────────────────────
SERVER_LOG="/tmp/talkgo-server-${TIMESTAMP}.log"
info "Starting server (log: ${SERVER_LOG})..."
cd "$PROJECT_DIR"
"${SERVER_BIN}" > "$SERVER_LOG" 2>&1 &
SERVER_PID=$!
info "Server PID: ${SERVER_PID}"

# Wait for server to be ready (up to 10 seconds).
info "Waiting for server to be ready..."
for i in $(seq 1 20); do
    if curl -s -o /dev/null -w '%{http_code}' http://localhost:8080/health 2>/dev/null | grep -q 200; then
        ok "Server is ready (attempt $i)."
        break
    fi
    if [[ $i -eq 20 ]]; then
        error "Server did not start within 10 seconds."
        exit 1
    fi
    sleep 0.5
done

# ── Step 4: Run loadgen ────────────────────────────────────────────────────
LOADGEN_LOG="/tmp/loadgen-${TIMESTAMP}.json"
info "Running loadgen for ${DURATION}..."
cd "$PROJECT_DIR"
"${LOADGEN_BIN}" -server localhost:8080 -duration "$DURATION" -profile "$PROFILE" -output "$LOADGEN_LOG"
LOADGEN_EXIT=$?

if [[ $LOADGEN_EXIT -ne 0 ]]; then
    warn "Loadgen exited with code ${LOADGEN_EXIT}."
fi

# Read loadgen report.
if [[ -f "$LOADGEN_LOG" ]]; then
    LOADGEN_REPORT=$(cat "$LOADGEN_LOG")
    ok "Loadgen report captured."
else
    warn "Loadgen report not found."
    LOADGEN_REPORT="{}"
fi

# ── Step 5: Parse server logs for chunk_latency ────────────────────────────
SERVER_STATS="{}"
if [[ $HAS_JQ -eq 1 && -f "$SERVER_LOG" ]]; then
    info "Parsing server logs..."

    TOTAL_CHUNKS=$(jq -s '[.[] | select(.msg == "chunk_latency") | .total_ms] | length' "$SERVER_LOG" 2>/dev/null || echo 0)
    CHUNKS_OK=$(jq -s '[.[] | select(.msg == "chunk_latency" and .status == "ok") | .total_ms] | length' "$SERVER_LOG" 2>/dev/null || echo 0)
    CHUNKS_ERROR=$(jq -s '[.[] | select(.msg == "chunk_latency" and .status == "error") | .total_ms] | length' "$SERVER_LOG" 2>/dev/null || echo 0)

    LATENCIES=$(jq -s '[.[] | select(.msg == "chunk_latency") | .total_ms] | sort' "$SERVER_LOG" 2>/dev/null || echo "[]")
    LATENCY_COUNT=$(echo "$LATENCIES" | jq 'length' 2>/dev/null || echo 0)

    if [[ "$LATENCY_COUNT" -gt 0 ]]; then
        LATENCY_MIN=$(echo "$LATENCIES" | jq 'min' 2>/dev/null || echo 0)
        LATENCY_MAX=$(echo "$LATENCIES" | jq 'max' 2>/dev/null || echo 0)
        LATENCY_P50=$(echo "$LATENCIES" | jq '.[(length * 50 / 100 | floor)]' 2>/dev/null || echo 0)
        LATENCY_P90=$(echo "$LATENCIES" | jq '.[(length * 90 / 100 | floor)]' 2>/dev/null || echo 0)
    else
        LATENCY_MIN=0; LATENCY_MAX=0; LATENCY_P50=0; LATENCY_P90=0
    fi

    # Parse AtoB/BtoA counts.
    CHUNKS_ATOB=$(jq -s '[.[] | select(.msg == "chunk_latency" and .half == "AtoB") | .total_ms] | length' "$SERVER_LOG" 2>/dev/null || echo 0)
    CHUNKS_BTOA=$(jq -s '[.[] | select(.msg == "chunk_latency" and .half == "BtoA") | .total_ms] | length' "$SERVER_LOG" 2>/dev/null || echo 0)

    # Error rate.
    if [[ "$TOTAL_CHUNKS" -gt 0 ]]; then
        ERROR_RATE=$(echo "scale=2; $CHUNKS_ERROR * 100 / $TOTAL_CHUNKS" | bc 2>/dev/null || echo "0")
    else
        ERROR_RATE="0"
    fi

    SERVER_STATS=$(cat <<JSON
{
  "total_chunks": ${TOTAL_CHUNKS},
  "chunks_ok": ${CHUNKS_OK},
  "chunks_error": ${CHUNKS_ERROR},
  "error_rate_pct": ${ERROR_RATE},
  "latency_p50_ms": ${LATENCY_P50},
  "latency_p90_ms": ${LATENCY_P90},
  "min_chunk_ms": ${LATENCY_MIN},
  "max_chunk_ms": ${LATENCY_MAX},
  "total_chunks_AtoB": ${CHUNKS_ATOB},
  "total_chunks_BtoA": ${CHUNKS_BTOA}
}
JSON
    )
    ok "Server logs parsed."
else
    warn "Server logs not parsed (jq unavailable or log file missing)."
fi

# ── Step 6: Generate consolidated report ────────────────────────────────────
info "Generating consolidated report..."

# Extract loadgen fields.
LG_RTT_AVG=$(echo "$LOADGEN_REPORT" | jq -r '.avg_rtt_ms // 0' 2>/dev/null || echo 0)
LG_RTT_P50=$(echo "$LOADGEN_REPORT" | jq -r '.p50_rtt_ms // 0' 2>/dev/null || echo 0)
LG_RTT_P90=$(echo "$LOADGEN_REPORT" | jq -r '.p90_rtt_ms // 0' 2>/dev/null || echo 0)
LG_LOSS=$(echo "$LOADGEN_REPORT" | jq -r '.packet_loss_pct // 0' 2>/dev/null || echo 0)
LG_STATUS=$(echo "$LOADGEN_REPORT" | jq -r '.status // "failed"' 2>/dev/null || echo "failed")

# Determine overall status.
# If loadgen failed -> overall failed.
OVERALL_STATUS="ok"
NOTES=()

if [[ "$LG_STATUS" == "failed" ]]; then
    OVERALL_STATUS="failed"
    NOTES+=("loadgen reported failure")
fi

# If server error rate > 15% or p90 > 2500 -> failed.
if (( $(echo "$ERROR_RATE > 15" | bc -l 2>/dev/null || echo 0) )); then
    OVERALL_STATUS="failed"
    NOTES+=("server error rate ${ERROR_RATE}% exceeds 15% threshold")
elif (( $(echo "$LATENCY_P90 > 2500" | bc -l 2>/dev/null || echo 0) )); then
    OVERALL_STATUS="failed"
    NOTES+=("server p90 latency ${LATENCY_P90}ms exceeds 2500ms threshold")
# If server error rate 5-15% or p90 1500-2500 -> degraded.
elif (( $(echo "$ERROR_RATE > 5" | bc -l 2>/dev/null || echo 0) )); then
    OVERALL_STATUS="degraded"
    NOTES+=("server error rate ${ERROR_RATE}% exceeds 5% threshold")
elif (( $(echo "$LATENCY_P90 > 1500" | bc -l 2>/dev/null || echo 0) )); then
    OVERALL_STATUS="degraded"
    NOTES+=("server p90 latency ${LATENCY_P90}ms exceeds 1500ms threshold")
fi

# Format notes as JSON array.
NOTES_JSON="[]"
if [[ ${#NOTES[@]} -gt 0 ]]; then
    NOTES_JSON=$(printf '%s\n' "${NOTES[@]}" | jq -R . | jq -s .)
fi

REPORT=$(cat <<JSON
{
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "profile": "${PROFILE}",
  "duration_sec": $(echo "$DURATION" | sed 's/s$//'),
  "server_logs": ${SERVER_STATS},
  "loadgen": {
    "avg_rtt_ms": ${LG_RTT_AVG},
    "p50_rtt_ms": ${LG_RTT_P50},
    "p90_rtt_ms": ${LG_RTT_P90},
    "packet_loss_pct": ${LG_LOSS}
  },
  "status": "${OVERALL_STATUS}",
  "notes": ${NOTES_JSON}
}
JSON
)

# ── Step 7: Output report ──────────────────────────────────────────────────
echo "$REPORT" | jq '.' > "$OUTPUT"
ok "Report written to ${OUTPUT}"
echo ""
echo "$REPORT" | jq '.'
echo ""
info "Results: status=${OVERALL_STATUS}, profile=${PROFILE}, duration=${DURATION}"
info "Server: ${CHUNKS_OK} ok / ${CHUNKS_ERROR} error chunks, p50=${LATENCY_P50}ms, p90=${LATENCY_P90}ms"
info "Loadgen: avg_rtt=${LG_RTT_AVG}ms, p50=${LG_RTT_P50}ms, p90=${LG_RTT_P90}ms, loss=${LG_LOSS}%"

if [[ "$OVERALL_STATUS" == "failed" ]]; then
    exit 1
fi
exit 0
