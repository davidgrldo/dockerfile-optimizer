FROM golang:1.24 AS build
RUN go build -o /app
FROM scratch
COPY --from=build /app /app
