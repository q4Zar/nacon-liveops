FROM golang:1.23 as builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o liveops ./cmd/server

FROM alpine:latest
RUN apk add --no-cache libc6-compat
WORKDIR /root/
COPY --from=builder /app/liveops .
EXPOSE 8080
CMD ["./liveops"]