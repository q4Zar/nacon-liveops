# Builder stage
FROM golang:1.23 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o liveops ./cmd/server
RUN CGO_ENABLED=1 GOOS=linux go build -o stressclient ./cmd/stressclient
RUN CGO_ENABLED=1 GOOS=linux go build -o liveops-cli ./cmd/cli

# Server stage
FROM debian:bookworm-slim AS server
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /root/
COPY --from=builder /app/liveops .
EXPOSE 8080
CMD ["./liveops"]

# Stressclient stage
FROM debian:bookworm-slim AS stressclient
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /root/
COPY --from=builder /app/stressclient .
CMD ["./stressclient"]

# CLI stage
FROM debian:bookworm-slim AS cli
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /root/
COPY --from=builder /app/liveops-cli .
CMD ["./liveops-cli", "interact"]