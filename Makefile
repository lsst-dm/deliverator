all:
	CGO_ENABLED=0 go build -ldflags "-extldflags '-static'" -o s3daemon-go
