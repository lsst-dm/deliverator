package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"html"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/conduitio/bwlimit"
	"github.com/hyperledger/fabric/common/semaphore"
	"golang.org/x/sys/unix"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
)

type S3DConf struct {
	host                  *string
	port                  *int
	endpoint_url          *string
	maxParallelUploads    *int64
	uploadTimeout         *time.Duration
	queueTimeout          *time.Duration
	uploadTries           *int
	uploadPartsize        *k8sresource.Quantity
	uploadBwlimit         *k8sresource.Quantity
	uploadBwlimitInteral  bool
	uploadWriteBufferSize *k8sresource.Quantity
}

type S3DHandler struct {
	Conf            *S3DConf
	AwsConfig       *aws.Config
	S3Client        *s3.Client
	Uploader        *manager.Uploader
	ParallelUploads *semaphore.Semaphore
}

type S3DUploadTask struct {
	uri    *url.URL
	bucket *string
	key    *string
	file   *string
}

func (h *S3DHandler) UploadFileMultipart(ctx context.Context, task *S3DUploadTask) error {
	start := time.Now()
	file, err := os.Open(*task.file)
	if err != nil {
		log.Printf("upload %v:%v | Couldn't open file %v to upload because: %v\n", *task.bucket, *task.key, *task.file, err)
		return err
	}
	defer file.Close()

	maxAttempts := *h.Conf.uploadTries
	var attempt int
	for attempt = 1; attempt <= maxAttempts; attempt++ {
		uploadCtx, cancel := context.WithTimeout(ctx, *h.Conf.uploadTimeout)
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

func (h *S3DHandler) parseRequest(r *http.Request) (*S3DUploadTask, error) {
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

	return &S3DUploadTask{uri: uri, bucket: &bucket, key: &key, file: &file}, nil
}

func (h *S3DHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	semaCtx, cancel := context.WithTimeout(r.Context(), *h.Conf.queueTimeout)
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

func getConf() S3DConf {
	conf := S3DConf{}

	// start flags
	conf.host = flag.String("host", os.Getenv("S3DAEMON_HOST"), "S3 Daemon Host (S3DAEMON_HOST)")

	defaultPort, _ := strconv.Atoi(os.Getenv("S3DAEMON_PORT"))
	if defaultPort == 0 {
		defaultPort = 15555
	}
	conf.port = flag.Int("port", defaultPort, "S3 Daemon Port (S3DAEMON_PORT)")

	conf.endpoint_url = flag.String("s3-endpoint-url", os.Getenv("S3_ENDPOINT_URL"), "S3 Endpoint URL (S3_ENDPOINT_URL)")

	var defaultMaxParallelUploads int64
	defaultMaxParallelUploads, _ = strconv.ParseInt(os.Getenv("S3DAEMON_MAX_PARALLEL_UPLOADS"), 10, 64)
	if defaultMaxParallelUploads == 0 {
		defaultMaxParallelUploads = 100
	}
	conf.maxParallelUploads = flag.Int64("max-parallel-uploads", defaultMaxParallelUploads, "Max Parallel Uploads (S3DAEMON_MAX_PARALLEL_UPLOADS)")

	defaultUploadTimeout := os.Getenv("S3DAEMON_UPLOAD_TIMEOUT")
	if defaultUploadTimeout == "" {
		defaultUploadTimeout = "10s"
	}
	uploadTimeout := flag.String("upload-timeout", defaultUploadTimeout, "Upload Timeout (S3DAEMON_UPLOAD_TIMEOUT)")

	defaultQueueTimeout := os.Getenv("S3DAEMON_QUEUE_TIMEOUT")
	if defaultQueueTimeout == "" {
		defaultQueueTimeout = "10s"
	}
	queueTimeout := flag.String("queue-timeout", defaultQueueTimeout, "Queue Timeout waiting for transfer to start (S3DAEMON_QUEUE_TIMEOUT)")

	defaultUploadTries, _ := strconv.Atoi(os.Getenv("S3DAEMON_UPLOAD_TRIES"))
	if defaultUploadTries == 0 {
		defaultUploadTries = 1
	}
	conf.uploadTries = flag.Int("upload-tries", defaultUploadTries, "Max number of upload tries (S3DAEMON_UPLOAD_TRIES)")

	defaultUploadPartsize := os.Getenv("S3DAEMON_UPLOAD_PARTSIZE")
	if defaultUploadPartsize == "" {
		defaultUploadPartsize = "5Mi"
	}
	uploadPartsizeRaw := flag.String("upload-partsize", defaultUploadPartsize, "Upload Part Size (S3DAEMON_UPLOAD_PARTSIZE)")

	defaultUploadBwlimit := os.Getenv("S3DAEMON_UPLOAD_BWLIMIT")
	if defaultUploadBwlimit == "" {
		defaultUploadBwlimit = "0"
	}
	uploadBwlimitRaw := flag.String("upload-bwlimit", defaultUploadBwlimit, "Upload bandwidth limit in bits per second (S3DAEMON_UPLOAD_BWLIMIT)")

	defaultUploadBwlimitInternal, _ := strconv.ParseBool(os.Getenv("S3DAEMON_UPLOAD_BWLIMIT_INTERNAL"))
	uploadBwlimitInternal := flag.Bool("upload-bwlimit-internal", defaultUploadBwlimitInternal, "Use internal tcp pacing instead of fq (S3DAEMON_UPLOAD_BWLIMIT_INTERNAL)")

	defaultUploadWriteBufferSize := os.Getenv("S3DAEMON_UPLOAD_WRITE_BUFFER_SIZE")
	if defaultUploadWriteBufferSize == "" {
		defaultUploadWriteBufferSize = "64Ki"
	}
	uploadWriteBufferSizeRaw := flag.String("upload-write-buffer-size", defaultUploadWriteBufferSize, "Upload Write Buffer Size (S3DAEMON_UPLOAD_WRITE_BUFFER_SIZE)")

	flag.Parse()
	// end flags

	if *conf.endpoint_url == "" {
		log.Fatal("S3_ENDPOINT_URL is required")
	}

	uploadTimeoutDuration, err := time.ParseDuration(*uploadTimeout)
	if err != nil {
		log.Fatal("S3DAEMON_UPLOAD_TIMEOUT is invalid")
	}
	conf.uploadTimeout = &uploadTimeoutDuration

	queueTimeoutDuration, err := time.ParseDuration(*queueTimeout)
	if err != nil {
		log.Fatal("S3DAEMON_QUEUE_TIMEOUT is invalid")
	}
	conf.queueTimeout = &queueTimeoutDuration

	uploadPartsize, err := k8sresource.ParseQuantity(*uploadPartsizeRaw)
	if err != nil {
		log.Fatal("S3DAEMON_UPLOAD_PARTSIZE is invalid")
	}
	conf.uploadPartsize = &uploadPartsize

	uploadBwlimit, err := k8sresource.ParseQuantity(*uploadBwlimitRaw)
	if err != nil {
		log.Fatal("S3DAEMON_UPLOAD_BWLIMIT is invalid")
	}
	conf.uploadBwlimit = &uploadBwlimit

	conf.uploadBwlimitInteral = *uploadBwlimitInternal

	uploadWriteBufferSize, err := k8sresource.ParseQuantity(*uploadWriteBufferSizeRaw)
	if err != nil {
		log.Fatal("S3DAEMON_UPLOAD_WRITE_BUFFER_SIZE is invalid")
	}
	conf.uploadWriteBufferSize = &uploadWriteBufferSize

	log.Println("S3DAEMON_HOST:", *conf.host)
	log.Println("S3DAEMON_PORT:", *conf.port)
	log.Println("S3DAEMON_ENDPOINT_URL:", *conf.endpoint_url)
	log.Println("S3DAEMON_MAX_PARALLEL_UPLOADS:", *conf.maxParallelUploads)
	log.Println("S3DAEMON_UPLOAD_TIMEOUT:", *conf.uploadTimeout)
	log.Println("S3DAEMON_QUEUE_TIMEOUT:", *conf.queueTimeout)
	log.Println("S3DAEMON_UPLOAD_TRIES:", *conf.uploadTries)
	log.Println("S3DAEMON_UPLOAD_PARTSIZE:", conf.uploadPartsize.String())
	log.Println("S3DAEMON_UPLOAD_BWLIMIT:", conf.uploadBwlimit.String())
	log.Println("S3DAEMON_UPLOAD_BWLIMIT_INTERNAL:", conf.uploadBwlimitInteral)
	log.Println("S3DAEMON_UPLOAD_WRITE_BUFFER_SIZE:", conf.uploadWriteBufferSize.String())

	return conf
}

func NewHandler(conf *S3DConf) *S3DHandler {
	handler := &S3DHandler{
		Conf: conf,
	}

	maxConns := int(*conf.maxParallelUploads * 5) // allow for multipart upload creation

	var httpClient *awshttp.BuildableClient

	if conf.uploadBwlimit.Value() != 0 {
		var dialCtx func(ctx context.Context, network, address string) (net.Conn, error)

		if conf.uploadBwlimitInteral {
			dialCtx = bwlimit.NewDialer(&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 0,
			}, bwlimit.Byte(conf.uploadBwlimit.Value()/8), 0).DialContext
		} else {
			dialer := &net.Dialer{
				Control: func(network, address string, conn syscall.RawConn) error {
					// https://pkg.go.dev/syscall#RawConn
					var operr error
					if err := conn.Control(func(fd uintptr) {
						operr = syscall.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_MAX_PACING_RATE, int(conf.uploadBwlimit.Value()/8))
					}); err != nil {
						return err
					}
					return operr
				},
			}
			dialCtx = dialer.DialContext
		}

		httpClient = awshttp.NewBuildableClient().WithTransportOptions(func(t *http.Transport) {
			t.ExpectContinueTimeout = 0
			t.IdleConnTimeout = 0
			t.MaxIdleConns = maxConns
			t.MaxConnsPerHost = maxConns
			t.MaxIdleConnsPerHost = maxConns
			t.WriteBufferSize = int(conf.uploadWriteBufferSize.Value())
			// disable http/2 to prevent muxing over a single tcp connection
			t.ForceAttemptHTTP2 = false
			t.TLSClientConfig.NextProtos = []string{"http/1.1"}
			t.DialContext = dialCtx
		})
	} else {
		httpClient = awshttp.NewBuildableClient().WithTransportOptions(func(t *http.Transport) {
			t.ExpectContinueTimeout = 0
			t.IdleConnTimeout = 0
			t.MaxIdleConns = maxConns
			t.MaxConnsPerHost = maxConns
			t.MaxIdleConnsPerHost = maxConns
			t.WriteBufferSize = int(conf.uploadWriteBufferSize.Value())
			// disable http/2 to prevent muxing over a single tcp connection
			t.ForceAttemptHTTP2 = false
			t.TLSClientConfig.NextProtos = []string{"http/1.1"}
		})
	}

	awsCfg, err := config.LoadDefaultConfig(
		context.TODO(),
		config.WithBaseEndpoint(*conf.endpoint_url),
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
		u.PartSize = conf.uploadPartsize.Value()
	})

	sema := semaphore.New(int(*conf.maxParallelUploads))
	handler.ParallelUploads = &sema

	return handler
}

func main() {
	conf := getConf()

	handler := NewHandler(&conf)
	http.Handle("/", handler)

	addr := fmt.Sprintf("%s:%d", *conf.host, *conf.port)
	log.Println("Listening on", addr)

	err := http.ListenAndServe(addr, nil)
	if errors.Is(err, http.ErrServerClosed) {
		log.Printf("server closed\n")
	} else if err != nil {
		log.Fatal("error starting server: %s\n", err)
	}
}
