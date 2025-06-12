VERSION=$(shell git describe --tags --dirty --always)

LDFLAGS += -extldflags '-static'
LDFLAGS += -X github.com/lsst-dm/s3nd/version.Version=$(VERSION)

.PHONY: all
all: lint swagger build

.PHONY: build
build:
	swag init --pd
	CGO_ENABLED=0 go build -ldflags "${LDFLAGS}"
	strip s3nd

.PHONY: swagger
swagger:
	swagger generate markdown -f docs/swagger.yaml --output docs/swagger.md

.PHONY: lint
lint:
	golangci-lint run
