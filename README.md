# QPERF-GO

A performance measurement tool for QUIC similar to iperf.
Uses https://github.com/birneee/quic-go

## Build
```bash
go build
```

## Setup
It is recommended to increase the maximum buffer size by running (See https://github.com/quic-go/quic-go/wiki/UDP-Receive-Buffer-Size for details):

```bash
sysctl -w net.core.rmem_max=2500000
```

## Generate Self-signed certificate
```bash
openssl req -x509 -nodes -days 358000 -out server.crt -keyout server.key -config server.req
```

# 设置http3服务器

使用--www 指定静态文件目录

```
./bin/qperf-go server --port=8888 --http3 --www www 
```