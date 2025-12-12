# Build Stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# 1️⃣ 这一步是关键：设置国内可访问的 Go 代理（七牛云或阿里云），防止下载失败
ENV GOPROXY=https://goproxy.cn,direct

COPY go.mod ./
# 有时候还需要 go.sum，如果没有也行，但最好有
# COPY go.sum ./ 

RUN go mod download

COPY . .

# 编译
RUN CGO_ENABLED=0 GOOS=linux go build -o bot ./cmd/bot

# Run Stage
FROM alpine:latest

WORKDIR /root/
RUN apk --no-cache add ca-certificates tzdata

COPY --from=builder /app/bot .

CMD ["./bot"]