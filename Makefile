VERSION=$(shell git describe --tags --dirty --always)

LDFLAGS += -extldflags '-static' -w
LDFLAGS += -X github.com/lsst-dm/s3nd/version.Version=$(VERSION)

.PHONY: all
all: lint docs build

.PHONY: build
build:
	CGO_ENABLED=0 go build -ldflags "${LDFLAGS}"

.PHONY: docs
docs: swag swagger

.PHONY: swag
swag:
	swag init --pd

.PHONY: swagger
swagger:
	swagger generate markdown -f docs/swagger.yaml --output docs/swagger.md

.PHONY: lint
lint:
	golangci-lint run
