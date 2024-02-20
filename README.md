# QPERF-GO


## throughout

```
./bin/qperf-go server --port=8080 
./bin/qperf-go client --log-prefix=test --addr="127.0.0.1:8080" --t=60 
```

## http3 server for plt test
启动http3:
```
./bin/qperf-go server --port=8888 --http3 --www www 
```

加载客户端请求：
```
./bin/qperf-go client --http3 --quiet=True  https://xxx.xxx/xx https://xxx.xxx/xx
```