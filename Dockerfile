# syntax=docker/dockerfile:1.7

# ---- build stage ----
FROM golang:1.26-alpine AS build
WORKDIR /src

# Cache modules first
COPY go.mod go.sum* ./
RUN go mod download

# Copy source
COPY . .

# Resolve build version
ARG BUILD_VERSION=""
RUN set -eux; \
    if [ -z "${BUILD_VERSION}" ] && [ -f VERSION ]; then \
        BUILD_VERSION="$(tr -d '[:space:]' < VERSION)"; \
    fi

# Build static binary (no cgo, no VCS info, stripped)
RUN set -eux; \
    CGO_ENABLED=0 GOOS=linux go build \
        -buildvcs=false \
        -trimpath \
        -ldflags="-s -w -X ds2api/internal/version.BuildVersion=${BUILD_VERSION}" \
        -o /out/ds2api ./cmd/ds2api

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot AS runtime
WORKDIR /app
COPY --from=build /out/ds2api /usr/local/bin/ds2api
COPY --from=build /src/config.example.json /app/config.example.json
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/ds2api"]
