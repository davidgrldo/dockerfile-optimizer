# syntax=docker/dockerfile:1
ARG GO_VERSION=1.24
FROM golang:${GO_VERSION} AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build \
    -ldflags="-s -w" \
    -o /app ./cmd/app
# dockopt:disable GEN010
FROM gcr.io/distroless/static:nonroot
COPY --from=build /app /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]
