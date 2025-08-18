FROM golang:1.25.0-alpine AS builder

RUN apk --update --no-cache add \
    binutils \
    make \
    git \
    && rm -rf /root/.cache
WORKDIR /go/src/github.com/lsst-dm/s3nd
COPY . .
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.4 && make build

FROM alpine:3
WORKDIR /root/
COPY --from=builder /go/src/github.com/lsst-dm/s3nd/s3nd /bin/s3nd
ENTRYPOINT ["/bin/s3nd"]
