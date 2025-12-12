# Build Stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# 安装 git 和证书（下载依赖必须）
RUN apk add --no-cache git ca-certificates

# 设置国内代理，加速下载（可选，但在国内或CI环境非常推荐）
ENV GOPROXY=https://goproxy.cn,direct

# 1. 复制 go.mod
COPY go.mod ./

# 🔥 核心修复：创建一个空的 go.sum，然后用 tidy 自动补全
RUN touch go.sum
RUN go mod tidy

# 2. 复制剩下的代码
COPY . .

# 3. 编译
RUN CGO_ENABLED=0 GOOS=linux go build -o bot ./cmd/bot

# Run Stage
FROM alpine:latest

WORKDIR /root/
RUN apk --no-cache add ca-certificates tzdata

COPY --from=builder /app/bot .

CMD ["./bot"]
