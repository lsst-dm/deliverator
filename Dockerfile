FROM golang:1.22-alpine as builder

ARG BIN=s3nd
RUN apk --update --no-cache add \
    binutils \
    && rm -rf /root/.cache
WORKDIR /go/src/github.com/lsst-dm/s3nd
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-extldflags '-static'" -o s3nd && strip "$BIN"

FROM alpine:3
WORKDIR /root/
COPY --from=builder /go/src/github.com/lsst-dm/s3nd/$BIN /bin/$BIN
ENTRYPOINT ["/bin/s3nd"]
