VERSION=$(shell git describe --tags --dirty --always)

LDFLAGS += -extldflags '-static'
LDFLAGS += -X github.com/lsst-dm/s3nd/version.Version=$(VERSION)

all:
	swag init --pd
	CGO_ENABLED=0 go build -ldflags "${LDFLAGS}" -o s3nd

swagger:
	swagger generate markdown -f docs/swagger.yaml --output docs/swagger.md
