# ── Stage 1: Builder ────────────────────────────────────────────────────────
# Use bookworm (not alpine) because hraban/opus requires CGO + libopus-dev.
FROM golang:1.24-bookworm AS builder

WORKDIR /build

# Install CGO dependencies.
RUN apt-get update && apt-get install -y --no-install-recommends \
    libopus-dev \
    libopusfile-dev \
    pkg-config \
 && rm -rf /var/lib/apt/lists/*

# Cache Go module downloads as a separate layer.
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build.
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o /talkgo ./cmd/server

# ── Stage 2: Runtime ────────────────────────────────────────────────────────
# Use debian:bookworm-slim (not scratch) — the binary needs libopus.so at runtime.
FROM debian:bookworm-slim

WORKDIR /

# Install runtime shared libraries.
RUN apt-get update && apt-get install -y --no-install-recommends \
    libopus0 \
    libopusfile0 \
    ca-certificates \
 && rm -rf /var/lib/apt/lists/*

COPY --from=builder /talkgo /talkgo

EXPOSE 8080

ENTRYPOINT ["/talkgo"]
