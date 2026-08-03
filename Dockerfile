# ── Stage 1: Build ─────────────────────────────────────────────
FROM golang:1.25.4-alpine AS builder

WORKDIR /app

# Cache dependencies — only re-download if go.mod/go.sum change
COPY go.mod go.sum ./
RUN go mod download

# Build the binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server .

# ── Stage 2: Run ──────────────────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy binary + static assets + templates
COPY --from=builder /app/server .
COPY --from=builder /app/static ./static
COPY --from=builder /app/templates ./templates

EXPOSE 8080

CMD ["./server"]
