# Build stage — compiles both binaries
FROM golang:1.23-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /worker ./cmd/worker

# API runtime
FROM gcr.io/distroless/static-debian12 AS api

COPY --from=builder /api /api

EXPOSE 8080
ENTRYPOINT ["/api"]

# Worker runtime
FROM gcr.io/distroless/static-debian12 AS worker

COPY --from=builder /worker /worker

ENTRYPOINT ["/worker"]
