# client
./iperf3 -c 192.168.110.21 -p 1235 -t 10 --logfile ../log/tcp-0loss-5mbps.txt
./iperf3 -c 192.168.110.21 -p 1235 -t 10 --logfile ../log/tcp-0loss-10mbps.txt

# server
./iperf3 -s -p 1234