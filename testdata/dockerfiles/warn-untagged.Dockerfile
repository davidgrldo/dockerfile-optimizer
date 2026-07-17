FROM golang:1.24 AS builder
RUN CGO_ENABLED=0 go build -o /app ./cmd/app
FROM ubuntu
COPY --from=builder /app /app
ENTRYPOINT ["/app"]
