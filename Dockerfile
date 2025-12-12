# Build Stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# 安装 git 和证书（必须）
RUN apk add --no-cache git ca-certificates

# 设置国内代理，加速且稳定
ENV GOPROXY=https://goproxy.cn,direct

# 只复制 go.mod，暂时不复制 go.sum
COPY go.mod ./

# ⚠️ 关键修改：先创建一个空的 go.sum，防止报错
RUN touch go.sum

# ⚠️ 关键修改：用 go mod tidy 自动拉取依赖并生成 go.sum，代替 go mod download
RUN go mod tidy

# 复制源码
COPY . .

# 编译
RUN CGO_ENABLED=0 GOOS=linux go build -o bot ./cmd/bot

# Run Stage
FROM alpine:latest

WORKDIR /root/
RUN apk --no-cache add ca-certificates tzdata

COPY --from=builder /app/bot .

CMD ["./bot"]