FROM golang:1.21-alpine AS builder

WORKDIR /build

COPY . .

RUN go build -o nws_exporter

FROM alpine:latest

WORKDIR /app

COPY --from=builder /build/nws_exporter /app/nws_exporter

EXPOSE 8080

ENTRYPOINT ["/app/nws_exporter"]
