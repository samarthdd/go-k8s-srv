FROM golang:alpine AS builder
WORKDIR /go/src/github.com/k8-proxy/go-k8s-srv
COPY . .
RUN cd cmd \
    && env GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o  go-k8s-srv .

FROM alpine
COPY --from=builder /go/src/github.com/k8-proxy/go-k8s-srv/cmd/go-k8s-srv /bin/go-k8s-srv


ENTRYPOINT ["/bin/go-k8s-srv"]
