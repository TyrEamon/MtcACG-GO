FROM alpine:latest

WORKDIR /opt/manyacg/

RUN apk add --no-cache bash ca-certificates && update-ca-certificates

COPY manyacg .

RUN chmod +x manyacg

ENTRYPOINT ["./manyacg"]
