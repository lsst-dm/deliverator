package main

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/lsst-dm/s3nd/conf"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/hyperledger/fabric/common/semaphore"
	"golang.org/x/sys/unix"
)

type S3ndHandler struct {
	Conf            *conf.S3ndConf
	AwsConfig       *aws.Config
	S3Client        *s3.Client
	Uploader        *manager.Uploader
	ParallelUploads *semaphore.Semaphore
}

type S3ndUploadTask struct {
	uri    *url.URL
	bucket *string
	key    *string
	file   *string
}

func (h *S3ndHandler) UploadFileMultipart(ctx context.Context, task *S3ndUploadTask) error {
	start := time.Now()
	file, err := os.Open(*task.file)
	if err != nil {
		log.Printf("upload %v:%v | Couldn't open file %v to upload because: %v\n", *task.bucket, *task.key, *task.file, err)
		return err
	}
	defer file.Close()

	maxAttempts := *h.Conf.UploadTries
	var attempt int
	for attempt = 1; attempt <= maxAttempts; attempt++ {
		uploadCtx, cancel := context.WithTimeout(ctx, *h.Conf.UploadTimeout)
		defer cancel()
		_, err = h.Uploader.Upload(uploadCtx, &s3.PutObjectInput{
			Bucket: aws.String(*task.bucket),
			Key:    aws.String(*task.key),
			Body:   file,
		})
		if err != nil {
			log.Printf("upload %v:%v | failed after %s -- try %v/%v\n", *task.bucket, *task.key, time.Now().Sub(start), attempt, maxAttempts)
			var noBucket *types.NoSuchBucket
			if errors.As(err, &noBucket) {
				log.Printf("upload %v:%v | Bucket does not exist.\n", *task.bucket, *task.key)
				// Don't retry if the bucket doesn't exist.
				return noBucket
			}

			if errors.Is(err, context.Canceled) {
				log.Printf("upload %v:%v | context cancelled\n", *task.bucket, *task.key)
				// Don't retry if the client disconnected
				return err
			}

			log.Printf("upload %v:%v | failed because: %v\n", *task.bucket, *task.key, err)

			// bubble up the error if we've exhausted our attempts
			if attempt == maxAttempts {
				return err
			}
		} else {
			break
		}
	}

	log.Printf("upload %v:%v | success in %s after %v/%v tries\n", *task.bucket, *task.key, time.Now().Sub(start), attempt, maxAttempts)
	return nil
}

func (h *S3ndHandler) parseRequest(r *http.Request) (*S3ndUploadTask, error) {
	file := r.PostFormValue("file")
	if file == "" {
		return nil, fmt.Errorf("missing field: file")
	}
	uriRaw := r.PostFormValue("uri")
	if uriRaw == "" {
		return nil, fmt.Errorf("missing field: uri")
	}

	if !filepath.IsAbs(file) {
		return nil, fmt.Errorf("Only absolute file paths are supported: %q", html.EscapeString(file))
	}

	uri, err := url.Parse(uriRaw)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse URI: %q", html.EscapeString(uriRaw))
	}

	if uri.Scheme != "s3" {
		return nil, fmt.Errorf("Only s3 scheme is supported: %q", html.EscapeString(uriRaw))
	}

	bucket := uri.Host
	if bucket == "" {
		return nil, fmt.Errorf("Unable to parse bucket from URI: %q", html.EscapeString(uriRaw))
	}
	key := uri.Path[1:] // Remove leading slash

	return &S3ndUploadTask{uri: uri, bucket: &bucket, key: &key, file: &file}, nil
}

func (h *S3ndHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	task, err := h.parseRequest(r)
	if err != nil {
		w.Header().Set("x-error", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "error parsing request: %s\n", err)
		return
	}

	log.Printf("queuing %v:%v | source %v\n", *task.bucket, *task.key, *task.file)

	// limit the number of parallel uploads
	semaCtx, cancel := context.WithTimeout(r.Context(), *h.Conf.QueueTimeout)
	defer cancel()
	if err := h.ParallelUploads.Acquire(semaCtx); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "error acquiring semaphore: %s\n", err)
		log.Printf("queue %v:%v | failed after %s: %s\n", *task.bucket, *task.key, time.Now().Sub(start), err)
		return
	}
	defer h.ParallelUploads.Release()

	if err := h.UploadFileMultipart(r.Context(), task); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "error uploading file: %s\n", err)
		return
	}

	fmt.Fprintf(w, "Successful put %q\n", html.EscapeString(task.uri.String()))
}

func NewHandler(conf *conf.S3ndConf) *S3ndHandler {
	handler := &S3ndHandler{
		Conf: conf,
	}

	maxConns := int(*conf.UploadMaxParallel * 5) // allow for multipart upload creation

	var httpClient *awshttp.BuildableClient

	if conf.UploadBwlimit.Value() != 0 {
		dialer := &net.Dialer{
			Control: func(network, address string, conn syscall.RawConn) error {
				// https://pkg.go.dev/syscall#RawConn
				var operr error
				if err := conn.Control(func(fd uintptr) {
					operr = syscall.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_MAX_PACING_RATE, int(conf.UploadBwlimit.Value()/8))
				}); err != nil {
					return err
				}
				return operr
			},
		}

		httpClient = awshttp.NewBuildableClient().WithTransportOptions(func(t *http.Transport) {
			t.ExpectContinueTimeout = 0
			t.IdleConnTimeout = 0
			t.MaxIdleConns = maxConns
			t.MaxConnsPerHost = maxConns
			t.MaxIdleConnsPerHost = maxConns
			t.WriteBufferSize = int(conf.UploadWriteBufferSize.Value())
			// disable http/2 to prevent muxing over a single tcp connection
			t.ForceAttemptHTTP2 = false
			t.TLSClientConfig.NextProtos = []string{"http/1.1"}
			t.DialContext = dialer.DialContext
		})
	} else {
		httpClient = awshttp.NewBuildableClient().WithTransportOptions(func(t *http.Transport) {
			t.ExpectContinueTimeout = 0
			t.IdleConnTimeout = 0
			t.MaxIdleConns = maxConns
			t.MaxConnsPerHost = maxConns
			t.MaxIdleConnsPerHost = maxConns
			t.WriteBufferSize = int(conf.UploadWriteBufferSize.Value())
			// disable http/2 to prevent muxing over a single tcp connection
			t.ForceAttemptHTTP2 = false
			t.TLSClientConfig.NextProtos = []string{"http/1.1"}
		})
	}

	awsCfg, err := config.LoadDefaultConfig(
		context.TODO(),
		config.WithBaseEndpoint(*conf.EndpointUrl),
		config.WithHTTPClient(httpClient),
	)
	if err != nil {
		log.Fatal(err)
	}

	handler.AwsConfig = &awsCfg

	handler.S3Client = s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
		o.Retryer = aws.NopRetryer{} // we handle retries ourselves
	})

	handler.Uploader = manager.NewUploader(handler.S3Client, func(u *manager.Uploader) {
		u.Concurrency = 1000
		u.MaxUploadParts = 1000
		u.PartSize = conf.UploadPartsize.Value()
	})

	sema := semaphore.New(int(*conf.UploadMaxParallel))
	handler.ParallelUploads = &sema

	return handler
}

func main() {
	conf := conf.NewConf()

	handler := NewHandler(&conf)
	http.Handle("/", handler)

	addr := fmt.Sprintf("%s:%d", *conf.Host, *conf.Port)
	log.Println("Listening on", addr)

	err := http.ListenAndServe(addr, nil)
	if errors.Is(err, http.ErrServerClosed) {
		log.Printf("server closed\n")
	} else if err != nil {
		log.Fatalf("error starting server: %s\n", err)
	}
}
