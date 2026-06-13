#!/usr/bin/env bash
#
# simulate-4g.sh — Linux network condition simulator using tc and netem.
#
# Applies bandwidth, latency, packet loss, and jitter to a network interface
# using Linux traffic control (tc). Requires sudo and iproute2.
#
# Usage:
#   ./simulate-4g.sh -Profile 4g
#   ./simulate-4g.sh -Interface wlan0 -LatencyMs 150 -LossPct 3 -RateMbps 5
#   ./simulate-4g.sh -Reset
#   ./simulate-4g.sh -ShowProfiles
#
# Flags:
#   -Profile <name>   Load profile from configs/<name>.yml
#   -Interface <dev>  Network interface (default: eth0)
#   -LatencyMs <ms>   Additional latency in ms
#   -LossPct <%>      Packet loss percentage
#   -RateMbps <mbps>  Bandwidth limit in Mbps
#   -JitterMs <ms>    Jitter in ms (default: 10% of latency)
#   -Reset            Remove all tc rules on the interface
#   -ShowProfiles     List available profiles
#   -h, --help        Show this help message

set -euo pipefail

# ── Defaults ────────────────────────────────────────────────────────────────
IFACE="eth0"
LATENCY=""
LOSS=""
RATE=""
JITTER=""
PROFILE=""
RESET=0
SHOW_PROFILES=0

# ── Help ────────────────────────────────────────────────────────────────────
usage() {
    cat <<'HELP'
TalkGo Network Simulator — Linux (full tc/netem support)

USAGE:
  ./simulate-4g.sh -Profile <name>    Apply a network profile
  ./simulate-4g.sh -Reset             Remove all tc rules
  ./simulate-4g.sh -ShowProfiles      List available profiles

FLAGS:
  -Profile <name>    Profile name (4g, wifi-cafe, wifi-home, wan-lossy)
  -Interface <dev>   Network interface (default: eth0)
  -LatencyMs <ms>    Additional round-trip latency in ms
  -LossPct <%>       Packet loss percentage
  -RateMbps <mbps>   Bandwidth limit in Mbps
  -JitterMs <ms>     Jitter in ms (default: 10% of latency)
  -Reset             Remove all tc rules
  -ShowProfiles      List available profiles
  -h, --help         Show this help

REQUIREMENTS:
  - Linux with iproute2 (tc command)
  - sudo/root privileges
  - The target interface must exist

EXAMPLES:
  ./simulate-4g.sh -Profile 4g
  ./simulate-4g.sh -Interface wlan0 -LatencyMs 150 -LossPct 3 -RateMbps 5
  ./simulate-4g.sh -Reset
HELP
}

# ── Parse YAML value from flat config ──────────────────────────────────────
parse_yaml_value() {
    local key="$1" file="$2"
    if [[ ! -f "$file" ]]; then
        echo ""
        return
    fi
    grep "^${key}:" "$file" | awk '{print $2}' | tr -d '"'
}

# ── Parse flags ────────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
    case "$1" in
        -Profile)       PROFILE="$2"; shift 2 ;;
        -Interface)     IFACE="$2"; shift 2 ;;
        -LatencyMs)     LATENCY="$2"; shift 2 ;;
        -LossPct)       LOSS="$2"; shift 2 ;;
        -RateMbps)      RATE="$2"; shift 2 ;;
        -JitterMs)      JITTER="$2"; shift 2 ;;
        -Reset)         RESET=1; shift ;;
        -ShowProfiles)  SHOW_PROFILES=1; shift ;;
        -h|--help)      usage; exit 0 ;;
        *)              echo "Unknown flag: $1"; usage; exit 1 ;;
    esac
done

# ── Prerequisites check ────────────────────────────────────────────────────
if [[ $EUID -ne 0 ]]; then
    echo "ERROR: This script requires root/sudo privileges." >&2
    echo "Run: sudo ./simulate-4g.sh ..." >&2
    exit 1
fi

if ! command -v tc &>/dev/null; then
    echo "ERROR: 'tc' not found. Install iproute2:" >&2
    echo "  apt-get install iproute2  (Debian/Ubuntu)" >&2
    echo "  yum install iproute      (CentOS/RHEL)" >&2
    exit 1
fi

# ── Script directory ───────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_DIR="${SCRIPT_DIR}/configs"

# ── Show profiles ──────────────────────────────────────────────────────────
if [[ $SHOW_PROFILES -eq 1 ]]; then
    echo "Available network profiles:"
    shopt -s nullglob
    for f in "$CONFIG_DIR"/*.yml; do
        name="$(basename "$f" .yml)"
        desc="$(parse_yaml_value "description" "$f")"
        echo "  ${name} — ${desc}"
    done
    shopt -u nullglob
    exit 0
fi

# ── Load profile ───────────────────────────────────────────────────────────
if [[ -n "$PROFILE" ]]; then
    PROFILE_FILE="${CONFIG_DIR}/${PROFILE}.yml"
    if [[ ! -f "$PROFILE_FILE" ]]; then
        echo "ERROR: Profile '${PROFILE}' not found at ${PROFILE_FILE}" >&2
        echo "Use -ShowProfiles to list available profiles." >&2
        exit 1
    fi
    echo "Loading profile '${PROFILE}' from ${PROFILE_FILE}"

    # Only set values if not overridden by explicit flags.
    if [[ -z "$RATE" ]]; then
        RATE="$(parse_yaml_value "bandwidth_mbps" "$PROFILE_FILE")"
    fi
    if [[ -z "$LATENCY" ]]; then
        LATENCY="$(parse_yaml_value "rtt_ms" "$PROFILE_FILE")"
    fi
    if [[ -z "$LOSS" ]]; then
        LOSS="$(parse_yaml_value "loss_pct" "$PROFILE_FILE")"
    fi
    JITTER_VAL="$(parse_yaml_value "jitter_ms" "$PROFILE_FILE")"
    IFACE_VAL="$(parse_yaml_value "interface" "$PROFILE_FILE")"
    if [[ -z "$JITTER" && -n "$JITTER_VAL" ]]; then
        JITTER="$JITTER_VAL"
    fi
    if [[ -n "$IFACE_VAL" && "$IFACE_VAL" != "null" ]]; then
        IFACE="$IFACE_VAL"
    fi

    echo "  interface: ${IFACE}, rate: ${RATE}Mbps, latency: ${LATENCY}ms, loss: ${LOSS}%, jitter: ${JITTER}ms"
fi

# ── Reset ───────────────────────────────────────────────────────────────────
if [[ $RESET -eq 1 ]]; then
    echo "Resetting network settings on ${IFACE}..."
    tc qdisc del dev "${IFACE}" root 2>/dev/null || true
    echo "  tc rules removed from ${IFACE}"
    exit 0
fi

# ── Validate parameters ────────────────────────────────────────────────────
if [[ -z "$RATE" && -z "$LATENCY" && -z "$LOSS" ]]; then
    echo "ERROR: No parameters set. Use -Profile <name> or explicit flags." >&2
    echo "Run '${0} -h' for help." >&2
    exit 1
fi

# Default values when not specified.
RATE="${RATE:-10}"
LATENCY="${LATENCY:-0}"
LOSS="${LOSS:-0}"
JITTER="${JITTER:-$(( LATENCY / 10 ))}"

# ── Apply tc rules ─────────────────────────────────────────────────────────
echo "Applying network simulation on ${IFACE}:"
echo "  bandwidth: ${RATE}Mbps, latency: ${LATENCY}ms, loss: ${LOSS}%, jitter: ${JITTER}ms"

# Remove existing qdisc first.
tc qdisc del dev "${IFACE}" root 2>/dev/null || true

# Step 1: Add HTB root for bandwidth shaping.
tc qdisc add dev "${IFACE}" root handle 1: htb default 30
tc class add dev "${IFACE}" parent 1: classid 1:1 htb rate "${RATE}mbit" ceil "${RATE}mbit"

# Step 2: Add netem for latency, loss, and jitter.
if [[ "$LATENCY" -gt 0 || "$LOSS" -gt 0 || "$JITTER" -gt 0 ]]; then
    NETEM_ARGS=""
    if [[ "$LATENCY" -gt 0 ]]; then
        NETEM_ARGS+=" delay ${LATENCY}ms"
        if [[ "$JITTER" -gt 0 ]]; then
            NETEM_ARGS+=" ${JITTER}ms"
        fi
    fi
    if [[ "$LOSS" -gt 0 ]]; then
        NETEM_ARGS+=" loss ${LOSS}%"
    fi

    tc qdisc add dev "${IFACE}" parent 1:1 handle 10: netem${NETEM_ARGS}
fi

echo ""
echo "Done. Network simulation active on ${IFACE}."
echo "Run '${0} -Reset' to remove rules."
echo ""
echo "Verify with: tc qdisc show dev ${IFACE}"
echo "Measure with: ping -c 10 ${IFACE}"
