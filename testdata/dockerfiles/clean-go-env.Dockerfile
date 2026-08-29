FROM golang:1.24 AS build
ENV CGO_ENABLED=0
WORKDIR /src
COPY . .
RUN go build -o /app ./cmd/app
FROM scratch
COPY --from=build /app /app
USER 65532
ENTRYPOINT ["/app"]
