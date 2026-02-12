FROM golang:1.22-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o zframe main.go

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/zframe .
RUN mkdir /data
EXPOSE 8080
CMD ["./zframe"]