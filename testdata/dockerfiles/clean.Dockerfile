FROM golang:1.24 AS build
RUN CGO_ENABLED=0 go build -o /app
FROM scratch
COPY --from=build /app /app
