FROM golang:1.22-alpine AS builder

ENV CGO_ENABLED=0

WORKDIR /src
COPY go.sum go.sum
COPY go.mod go.mod
RUN go mod download
COPY . .
RUN go build -o nada-bucket-proxy .

FROM alpine:3

WORKDIR /app
COPY --from=builder /src/nada-bucket-proxy /app/nada-bucket-proxy
CMD ["/app/nada-bucket-proxy"]