FROM alpine:latest

WORKDIR /opt/manyacg/

# 只安装必要依赖（无 ffmpeg）
RUN apk add --no-cache bash ca-certificates && update-ca-certificates

COPY manyacg .

RUN chmod +x manyacg

ENTRYPOINT ["./manyacg"]
