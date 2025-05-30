FROM golang:1.24.3-alpine AS builder

RUN apk --update --no-cache add \
    binutils \
    make \
    git \
    && rm -rf /root/.cache
WORKDIR /go/src/github.com/lsst-dm/s3nd
COPY . .
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.4
RUN CGO_ENABLED=0 make && strip s3nd

FROM alpine:3
WORKDIR /root/
COPY --from=builder /go/src/github.com/lsst-dm/s3nd/s3nd /bin/s3nd
ENTRYPOINT ["/bin/s3nd"]
