# syntax=docker/dockerfile:1

ARG GO_VERSION=1.27

# ---- build stage ----
FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/gateway ./cmd/gateway

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/gateway /gateway

ENV LISTEN_ADDR=:8080
EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/gateway"]
