# iperf
./qperf-go server --port 8888
./qperf-go client --addr 127.0.0.1:8888 -t 30



# http3

# server
./qperf-go server --port 8888 --http3 --www www

# client
./qperf-go client --http3 --quiet=True https://127.0.0.1:8888/1.jpeg https://127.0.0.1:8888/index.html