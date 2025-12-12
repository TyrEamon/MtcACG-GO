# Build Stage (编译阶段)
FROM golang:1.21-alpine AS builder

WORKDIR /app

# 先复制依赖描述文件
COPY go.mod ./
# 这一步会自动下载依赖 (你本地不用做)
RUN go mod download

# 复制剩下的所有代码
COPY . .

# 编译成可执行文件
RUN CGO_ENABLED=0 GOOS=linux go build -o bot ./cmd/bot

# Run Stage (运行阶段)
FROM alpine:latest

WORKDIR /root/
# 安装证书 (HTTPS请求必须)
RUN apk --no-cache add ca-certificates tzdata

# 从第一阶段复制编译好的程序
COPY --from=builder /app/bot .

# 启动
CMD ["./bot"]