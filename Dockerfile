# Multi-stage production build for KubeHealer
# Stage 1: Build the static Go binary
FROM golang:1.22-alpine AS builder

WORKDIR /src

# Cache dependency layer
COPY go.mod ./
RUN go mod download || true

# Copy source code
COPY . .

# Compile optimized static binary (CGO_ENABLED=0 eliminates glibc dynamic linkage)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.version=1.0.0 -X main.commit=$(git rev-parse --short HEAD 2>/dev/null || echo 'release')" \
    -o /bin/kubehealer ./cmd/kubehealer

# Stage 2: Minimal hardened runtime image
FROM alpine:3.20

RUN apk --no-cache add ca-certificates tzdata curl && \
    addgroup -S -g 10001 nonroot && \
    adduser -S -u 10001 -G nonroot -h /app nonroot

WORKDIR /app
COPY --from=builder /bin/kubehealer /app/kubehealer

USER nonroot:nonroot

EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/app/kubehealer"]
CMD ["--mode=daemon", "--port=8080"]
