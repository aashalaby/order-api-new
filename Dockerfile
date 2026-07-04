# syntax=docker/dockerfile:1

# ==========================================
# STAGE 1: Build the Go Binary
# ==========================================
# --platform=$BUILDPLATFORM: the builder ALWAYS runs natively on the CI runner's
# architecture. Without this, the arm64 build runs the whole Go toolchain under
# QEMU emulation (5-10x slower). Go cross-compiles natively instead.
#
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

# Injected automatically by docker buildx for each target platform
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

# Copy dependency manifests first for layer caching
COPY go.mod go.sum ./

# Cache mount for the module cache: survives across builds even when
# go.mod changes, so only NEW modules are downloaded
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

# Cross-compile natively for the target platform.
# - CGO_ENABLED=0    : fully static binary, no libc dependency
# - -trimpath        : strips local filesystem paths (smaller, reproducible)
# - -ldflags "-s -w" : strips symbol table and DWARF debug info
# - -buildvcs=false  : no .git in the build context, so disable VCS stamping
# The second cache mount keeps Go's incremental build cache, so unchanged
# packages are never recompiled.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/order-api .

# ==========================================
# STAGE 2: Minimal Runtime Image
# ==========================================
FROM cgr.dev/chainguard/static

WORKDIR /app

COPY --from=builder /out/order-api .

EXPOSE 8080

ENTRYPOINT ["./order-api"]
