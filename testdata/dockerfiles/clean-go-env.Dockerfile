# dockopt:disable GEN010
FROM golang:1.24 AS build
ENV CGO_ENABLED=0
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build go build -o /app ./cmd/app
FROM scratch
COPY --from=build /app /app
USER 65532
ENTRYPOINT ["/app"]
