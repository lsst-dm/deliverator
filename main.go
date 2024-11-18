package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/hyperledger/fabric/common/semaphore"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
)

type S3DConf struct {
	host         *string
	port         *int
	endpoint_url *string
	// access_key   *string
	// secret_key   *string
	maxParallelUploads *int64
	uploadTimeout      *time.Duration
	queueTimeout       *time.Duration
	uploadTries        *int
	uploadPartsize     *k8sresource.Quantity
}

type S3DHandler struct {
	Conf            *S3DConf
	AwsConfig       *aws.Config
	S3Client        *s3.Client
	Uploader        *manager.Uploader
	ParallelUploads *semaphore.Semaphore
}

// UploadObject uses the S3 upload manager to upload an object to a bucket.
func (h *S3DHandler) UploadFileMultipart(bucket string, key string, fileName string) error {
	start := time.Now()
	file, err := os.Open(fileName)
	if err != nil {
		log.Printf("upload %v:%v | Couldn't open file %v to upload because: %v\n", bucket, key, fileName, err)
		return err
	}
	defer file.Close()
	// data, err := ioutil.ReadFile(fileName)
	// log.Printf("slurped %v:%v in %s\n", bucket, key, time.Now().Sub(start))

	maxAttempts := *h.Conf.uploadTries
	var attempt int
	for attempt = 1; attempt <= maxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.TODO(), *h.Conf.uploadTimeout)
		defer cancel()
		_, err = h.Uploader.Upload(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
			// Body:   bytes.NewReader([]byte(data)),
			Body: file,
		})
		if err != nil {
			var noBucket *types.NoSuchBucket
			if errors.As(err, &noBucket) {
				log.Printf("upload %v:%v | Bucket %s does not exist.\n", bucket, key, bucket)
				return noBucket // Don't retry if the bucket doesn't exist.
			}

			log.Printf("upload %v:%v | failed after %s -- try %v/%v\n", bucket, key, time.Now().Sub(start), attempt, maxAttempts)
			log.Printf("upload %v:%v | failed because: %v\n", bucket, key, err)

			// bubble up the error if we've exhausted our attempts
			if attempt == maxAttempts {
				return err
			}
		} else {
			break
		}
	}

	log.Printf("upload %v:%v | success in %s after %v/%v tries\n", bucket, key, time.Now().Sub(start), attempt, maxAttempts)
	return nil
}

func (h *S3DHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	file := r.PostFormValue("file")
	if file == "" {
		w.Header().Set("x-missing-field", "file")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	uri := r.PostFormValue("uri")
	if uri == "" {
		w.Header().Set("x-missing-field", "uri")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if !filepath.IsAbs(file) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Only absolute file paths are supported, %q\n", html.EscapeString(file))
		return
	}

	u, err := url.Parse(uri)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Unable to parse URI, %q\n", html.EscapeString(uri))
		return
	}

	if u.Scheme != "s3" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Only s3 scheme is supported, %q\n", html.EscapeString(uri))
		return
	}

	bucket := u.Host
	if bucket == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Unable to parse bucket from URI, %q\n", html.EscapeString(uri))
		return
	}
	key := u.Path[1:] // Remove leading slash

	log.Printf("queuing %v:%v | source %v\n", bucket, key, file)

	// limit the number of parallel uploads
	ctx, cancel := context.WithTimeout(context.Background(), *h.Conf.queueTimeout)
	defer cancel()
	if err := h.ParallelUploads.Acquire(ctx); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "error acquiring semaphore: %s\n", err)
		log.Printf("queue %v:%v | failed after %s: %s\n", bucket, key, time.Now().Sub(start), err)
		return
	}
	defer h.ParallelUploads.Release()

	err = h.UploadFileMultipart(bucket, key, file)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "error uploading file: %s\n", err)
		return
	}

	fmt.Fprintf(w, "Successful put %q\n", html.EscapeString(uri))
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
	uploadPartsize := flag.String("upload-partsize", defaultUploadPartsize, "Upload Part Size (S3DAEMON_UPLOAD_PARTSIZE)")

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

	uploadPartsizeQuantity, err := k8sresource.ParseQuantity(*uploadPartsize)
	if err != nil {
		log.Fatal("S3DAEMON_UPLOAD_PARTSIZE is invalid")
	}
	conf.uploadPartsize = &uploadPartsizeQuantity

	log.Println("S3DAEMON_HOST:", *conf.host)
	log.Println("S3DAEMON_PORT:", *conf.port)
	log.Println("S3DAEMON_ENDPOINT_URL:", *conf.endpoint_url)
	log.Println("S3DAEMON_MAX_PARALLEL_UPLOADS:", *conf.maxParallelUploads)
	log.Println("S3DAEMON_UPLOAD_TIMEOUT:", *conf.uploadTimeout)
	log.Println("S3DAEMON_QUEUE_TIMEOUT:", *conf.queueTimeout)
	log.Println("S3DAEMON_UPLOAD_TRIES:", *conf.uploadTries)
	log.Println("S3DAEMON_UPLOAD_PARTSIZE:", conf.uploadPartsize.String())

	return conf
}

func NewHandler(conf *S3DConf) *S3DHandler {
	handler := &S3DHandler{
		Conf: conf,
	}

	maxConns := int(*conf.maxParallelUploads * 5) // allow for multipart upload creation

	httpClient := awshttp.NewBuildableClient().WithTransportOptions(func(t *http.Transport) {
		t.ExpectContinueTimeout = 0
		t.IdleConnTimeout = 0
		t.MaxIdleConns = maxConns
		t.MaxConnsPerHost = maxConns
		t.MaxIdleConnsPerHost = maxConns
		t.WriteBufferSize = 64 * 1024
		// disable http/2 to prevent muxing over a single tcp connection
		t.ForceAttemptHTTP2 = false
		t.TLSClientConfig.NextProtos = []string{"http/1.1"}
	})

	awsCfg, err := config.LoadDefaultConfig(
		context.TODO(),
		config.WithBaseEndpoint(*conf.endpoint_url),
		config.WithHTTPClient(httpClient),
		// config.WithRetryer(func() aws.Retryer {
		// 	return retry.NewStandard(func(o *retry.StandardOptions) {
		// 		o.MaxAttempts = 10
		// 		o.MaxBackoff = time.Millisecond * 500
		// 		o.RateLimiter = ratelimit.None
		// 	})
		// }),
	)
	if err != nil {
		log.Fatal(err)
	}

	handler.AwsConfig = &awsCfg

	handler.S3Client = s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
		o.Retryer = aws.NopRetryer{}
	})

	/*
		resp, err := s3Client.ListBuckets(context.TODO(), nil)
		if err != nil {
			log.Fatal(err)
		}

		// Print out the list of buckets
		log.Println("Buckets:")
		for _, bucket := range resp.Buckets {
			log.Println(*bucket.Name)
		}
	*/

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
