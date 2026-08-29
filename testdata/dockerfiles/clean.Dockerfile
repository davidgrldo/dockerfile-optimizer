# dockopt:disable GEN010
FROM golang:1.24 AS build
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -o /app
FROM scratch
COPY --from=build /app /app
USER 65532
