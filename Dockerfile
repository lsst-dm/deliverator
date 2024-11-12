FROM golang:1.22-alpine as builder

ARG BIN=s3daemon-go
RUN apk --update --no-cache add \
    binutils \
    && rm -rf /root/.cache
WORKDIR /go/src/github.com/jhoblitt/s3daemon-go
COPY . .
RUN go build && strip "$BIN"

FROM alpine:3
WORKDIR /root/
COPY --from=builder /go/src/github.com/jhoblitt/s3daemon-go/$BIN /bin/$BIN
ENTRYPOINT ["/bin/s3daemon-go"]
