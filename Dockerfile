FROM golang:1.19.7-bullseye
SHELL ["/bin/bash", "-c"]
COPY . /app
WORKDIR /app
RUN go env -w GO111MODULE=on && go env -w GOPROXY=https://goproxy.cn,direct && go install .
ENV PORT=8080
ENTRYPOINT exec /go/bin/qperf-go server --port ${PORT}
EXPOSE ${PORT}