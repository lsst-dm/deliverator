package upload

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/lsst-dm/deliverator/conf"
	"github.com/lsst-dm/deliverator/conntracker"
	"github.com/lsst-dm/deliverator/semaphore"
	"github.com/lsst-dm/deliverator/upload/badrequesterror"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithy "github.com/aws/smithy-go"
	"github.com/google/uuid"
	gherrors "github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	logger                  = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	errUploadAttemptTimeout = gherrors.New("upload attempt timeout")
	errUploadQueueTimeout   = gherrors.New("upload queue timeout")
	errUploadAborted        = gherrors.New("upload request aborted because the client disconnected")
	errUploadNoSuchBucket   = gherrors.New("upload failed because the bucket does not exist")
	uploadAttempts          = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "s3nd_upload_attempts_total",
			Help: "number of attempts to upload a file",
		},
	)
	uploadRetries = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "s3nd_upload_retries_total",
			Help: "number of attempts to upload a file after a failure",
		},
	)
	uploadRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "s3nd_upload_http_requests_total",
			Help: "http status codes returned to the client",
		},
		[]string{"code", "reason"},
	)
	uploadBytes = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "s3nd_upload_bytes_total",
			Help: "number of bytes transferred for files which completed successfully",
		},
	)
	s3HTTPResponses = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "s3nd_s3_http_responses_total",
			Help: "http status codes returned by the s3 service",
		},
		[]string{"code", "reason"},
	)
	s3APIErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "s3nd_s3_api_errors_total",
			Help: "api status codes returned by the s3 service",
		},
		[]string{"code", "reason"},
	)
)

type S3ndHandler struct {
	conf            *conf.S3ndConf
	awsConfig       *aws.Config
	s3Client        *s3.Client
	uploader        *manager.Uploader
	parallelUploads semaphore.Semaphore
	connTracker     *conntracker.ConnTracker
}

func (h *S3ndHandler) ConnTracker() *conntracker.ConnTracker {
	return h.connTracker
}

func (h *S3ndHandler) Conf() *conf.S3ndConf {
	return h.conf
}

func (h *S3ndHandler) ParallelUploads() *semaphore.Semaphore {
	return &h.parallelUploads
}

type UploadTask struct {
	Id                uuid.UUID   `json:"id" swaggertype:"string" format:"uuid"`
	Uri               *RequestURL `json:"uri,omitempty" swaggertype:"string" example:"s3://my-bucket/my-key"`
	Bucket            *string     `json:"-"`
	Key               *string     `json:"-"`
	File              *string     `json:"file,omitempty" swaggertype:"string" example:"/path/to/file.txt"`
	StartTime         time.Time   `json:"-"`
	EndTime           time.Time   `json:"-"`
	Duration          string      `json:"duration,omitempty" example:"21.916462ms"` // human friendly
	DurationSeconds   float64     `json:"duration_seconds,omitzero" example:"0.021"`
	Attempts          int         `json:"attempts,omitzero" example:"1"`
	SizeBytes         int64       `json:"size_bytes" example:"1000"`
	UploadParts       int64       `json:"upload_parts,omitempty" example:"1"`
	TransferRate      string      `json:"transfer_rate,omitempty" example:"1000B/s"` // human friendly
	TransferRateMbits float64     `json:"transfer_rate_mbits,omitzero" example:"0.001"`
	Slug              *string     `json:"slug,omitempty" example:"Gray Garden Slug"` // for logging
} //@name task

func NewUploadTask(startTime time.Time) *UploadTask {
	return &UploadTask{
		Id:        uuid.New(),
		StartTime: startTime,
	}
}

// the task is stopped because of an error and no data was sent
func (t *UploadTask) StopNoUpload() {
	t.Stop()
	// no transfer rate if we didn't start the upload
	t.TransferRate = ""
	t.TransferRateMbits = 0
}

func (t *UploadTask) Stop() {
	t.EndTime = time.Now()
	duration := t.EndTime.Sub(t.StartTime)
	t.DurationSeconds = duration.Seconds()
	t.Duration = duration.String()
	t.TransferRateMbits = float64(t.SizeBytes*8) / duration.Seconds() / (1 << 20)
	t.TransferRate = fmt.Sprintf("%.3fMbit/s", t.TransferRateMbits)
}

type RequestURL struct{ url.URL }

func (u RequestURL) MarshalText() ([]byte, error) {
	return []byte(u.String()), nil
}

func (u *RequestURL) UnmarshalText(text []byte) error {
	parsed, err := url.Parse(string(text))
	if err != nil {
		return err
	}
	u.URL = *parsed
	return nil
}

type RequestStatus struct {
	Code int         `json:"code" example:"200"`
	Msg  string      `json:"msg,omitempty" example:"upload succeeded"`
	Task *UploadTask `json:"task,omitempty"`
} //@name requestStatus200

// requestStatusSwag400 is used only for Swagger documentation
//
//nolint:unused
type requestStatusSwag400 struct {
	RequestStatus
	Code int    `json:"code" example:"400"`
	Msg  string `json:"msg,omitempty" example:"error parsing request: missing field: uri"`
	Task *struct {
		Id       uuid.UUID `json:"id" swaggertype:"string" format:"uuid"`
		File     *string   `json:"file,omitempty" swaggertype:"string" example:"/path/to/file.txt"`
		Duration string    `json:"duration,omitempty" example:"37.921µs"`
		Slug     string    `json:"slug,omitempty" example:"Gray Garden Slug"` // for logging
	} `json:"task,omitempty"`
} //@name requestStatus400

// requestStatusSwag404 is used only for Swagger documentation
//
//nolint:unused
type requestStatusSwag404 struct {
	RequestStatus
	Code int    `json:"code" example:"404"`
	Msg  string `json:"msg,omitempty" example:"upload failed because the bucket does not exist"`
} //@name requestStatus404

// requestStatusSwag408 is used only for Swagger documentation
//
//nolint:unused
type requestStatusSwag408 struct {
	RequestStatus
	Code int    `json:"code" example:"408"`
	Msg  string `json:"msg,omitempty" example:"upload queue timeout"`
} //@name requestStatus408

// requestStatusSwag500 is used only for Swagger documentation
//
//nolint:unused
type requestStatusSwag500 struct {
	RequestStatus
	Code int    `json:"code" example:"500"`
	Msg  string `json:"msg,omitempty" example:"unknown error"`
	Task *struct {
		UploadTask
		Duration string `json:"duration,omitempty" example:"37.921µs"`
		Attempts int    `json:"attempts,omitzero" example:"5"`
	} `json:"task,omitempty"`
} //@name requestStatus500

// requestStatusSwag504 is used only for Swagger documentation
//
//nolint:unused
type requestStatusSwag504 struct {
	RequestStatus
	Code int    `json:"code" example:"504"`
	Msg  string `json:"msg,omitempty" example:"timeout during upload attempt 2/2"`
	Task *struct {
		Id       uuid.UUID   `json:"id" swaggertype:"string" format:"uuid"`
		Uri      *RequestURL `json:"uri,omitempty" swaggertype:"string" example:"s3://my-bucket/my-key"`
		File     *string     `json:"file,omitempty" swaggertype:"string" example:"/path/to/file.txt"`
		Duration string      `json:"duration,omitempty" example:"56.115µs"`
		Slug     string      `json:"slug,omitempty" example:"Gray Garden Slug"` // for logging
	} `json:"task,omitempty"`
} //@name requestStatus504

func NewS3ndHandler(conf *conf.S3ndConf) *S3ndHandler {
	handler := &S3ndHandler{
		conf: conf,
	}

	maxConns := int(*conf.UploadMaxParallel)

	var httpClient *awshttp.BuildableClient

	defaultTransportOtptions := func(t *http.Transport) {
		t.ExpectContinueTimeout = 0
		t.IdleConnTimeout = 0
		t.MaxIdleConns = maxConns
		t.MaxConnsPerHost = maxConns
		t.MaxIdleConnsPerHost = maxConns
		t.WriteBufferSize = int(conf.UploadWriteBufferSize.Value())
		// disable http/2 to prevent muxing over a single tcp connection
		t.ForceAttemptHTTP2 = false
		t.TLSClientConfig.NextProtos = []string{"http/1.1"}
	}

	handler.connTracker = conntracker.NewConnTracker(&net.Dialer{})

	httpClient = awshttp.NewBuildableClient().WithTransportOptions(func(t *http.Transport) {
		defaultTransportOtptions(t)
		t.DialContext = handler.connTracker.DialContext
	})

	awsCfg, err := config.LoadDefaultConfig(
		context.TODO(),
		config.WithBaseEndpoint(*conf.EndpointUrl),
		config.WithHTTPClient(httpClient),
		config.WithRequestChecksumCalculation(aws.RequestChecksumCalculationWhenRequired),
	)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	handler.awsConfig = &awsCfg

	handler.s3Client = s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
		o.Retryer = aws.NopRetryer{} // we handle retries ourselves
	})

	handler.uploader = manager.NewUploader(handler.s3Client, func(u *manager.Uploader) {
		u.PartSize = conf.UploadPartsize.Value()
	})

	handler.parallelUploads = semaphore.New(int(*conf.UploadMaxParallel))

	// set the pacing rate before the first conn is established
	handler.updatePacingRate()

	return handler
}

// @Summary      upload file to S3
// @Tags         uploads
// @Accept       x-www-form-urlencoded
// @Produce      json
// @Param        uri  formData  string true  "Destination S3 URI"
// @Param        file formData  string true  "path to file to upload"
// @Param        slug formData  string false "arbitrary string to include in logs"
// @Success      200  {object}  RequestStatus
// @Failure      400  {object}  requestStatusSwag400
// @Failure      404  {object}  requestStatusSwag404
// @Failure      408  {object}  requestStatusSwag408
// @Failure      500  {object}  requestStatusSwag500
// @Failure      504  {object}  requestStatusSwag504
// @Router       /upload [post]
// @Header       400,404,408,500,504 {string} X-Error "error message"
func (h *S3ndHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	task, err := h.doServeHTTP(r)

	var code int
	var msg string
	var badRequestErr *badrequesterror.BadRequestError
	var smithyAPIErr smithy.APIError
	var awsHTTPError *awshttp.ResponseError

	awsError := func(err error) {
		if gherrors.As(err, &smithyAPIErr) {
			s3APIErrors.WithLabelValues(smithyAPIErr.ErrorCode(), smithyAPIErr.ErrorMessage()).Inc()
		}
		if gherrors.As(err, &awsHTTPError) {
			awsCode := awsHTTPError.HTTPStatusCode()
			s3HTTPResponses.WithLabelValues(strconv.Itoa(awsCode), http.StatusText(awsCode)).Inc()
		}
	}

	switch {
	case err == nil: // upload succeeded
		code = http.StatusOK
		msg = "upload succeeded"
		uploadBytes.Add(float64(task.SizeBytes))
	case gherrors.As(err, &badRequestErr):
		// bad request, e.g. missing required fields
		code = http.StatusBadRequest
		msg = err.Error()
	case gherrors.Is(err, errUploadAborted):
		// the socket was closed, so no http status code can be sent to the client.
		// Setting the status code is only for the purposes of logging/metrics.
		code = http.StatusTeapot
		msg = err.Error()
	case gherrors.Is(err, errUploadQueueTimeout):
		code = http.StatusRequestTimeout
		msg = err.Error()
	case gherrors.Is(err, errUploadAttemptTimeout):
		code = http.StatusGatewayTimeout
		msg = err.Error()
	case gherrors.Is(err, errUploadNoSuchBucket):
		code = http.StatusNotFound
		msg = err.Error()
		awsError(err)
	case gherrors.As(err, &smithyAPIErr):
		// aws sdk errors other than NoSuchBucket
		code = http.StatusBadGateway
		msg = err.Error()
		awsError(err)
	default:
		code = http.StatusInternalServerError
		msg = err.Error()
	}

	uploadRequests.WithLabelValues(strconv.Itoa(code), http.StatusText(code)).Inc()

	status := RequestStatus{
		Code: code,
		Msg:  msg,
		Task: task,
	}

	logLevel := slog.LevelInfo

	if status.Code != http.StatusOK {
		// it is an error response
		w.Header().Set("x-error", status.Msg)
		logLevel = slog.LevelError
	} else {
		// assume there was an http 200 response from s3
		s3HTTPResponses.WithLabelValues(strconv.Itoa(code), http.StatusText(code)).Inc()
	}

	logger.Log(
		r.Context(),
		logLevel,
		status.Msg,
		slog.Int("code", status.Code),
		slog.Any("task", status.Task),
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status.Code)
	_ = json.NewEncoder(w).Encode(status)
}

func (h *S3ndHandler) doServeHTTP(r *http.Request) (*UploadTask, error) {
	// create starting timestamp as early as possible
	task := NewUploadTask(time.Now())

	if err := h.parseRequest(task, r); err != nil {
		task.StopNoUpload()
		return task, err
	}

	logger.Info(
		"new upload request",
		slog.Any("task", task),
	)

	// limit the number of parallel uploads
	{
		logWait := func(ctx context.Context) error {
			logger.Info(
				"upload queued, waiting for upload slot(s)",
				slog.Any("task", task),
			)
			h.logUploads()

			return nil
		}

		ctx, cancel := context.WithTimeoutCause(r.Context(), *h.conf.QueueTimeout, errUploadQueueTimeout)
		defer cancel()

		if err := h.parallelUploads.Acquire(ctx, int(task.UploadParts), logWait); err != nil {
			task.StopNoUpload()
			if gherrors.Is(context.Cause(ctx), errUploadQueueTimeout) {
				err = errors.Join(errUploadQueueTimeout, err)
			} else if gherrors.Is(err, context.Canceled) {
				err = errors.Join(errUploadAborted, err)
			} else {
				// unknown error
				err = gherrors.Wrap(err, "unable to acquire upload queue semaphore")
			}
			return task, err
		}
	}
	defer h.updatePacingRate() // rebalance after semaphore weight is released
	defer h.parallelUploads.Release(int(task.UploadParts))

	logger.Info(
		"upload starting",
		slog.Any("task", task),
	)

	// set the packet pace when starting a new upload and after an upload is
	// finished (successfully or not)
	h.updatePacingRate()

	if err := h.uploadFileMultipart(r.Context(), task); err != nil {
		task.StopNoUpload()
		return task, err
	}

	task.Stop()

	return task, nil
}

func (h *S3ndHandler) parseRequest(task *UploadTask, r *http.Request) error {
	{
		file := r.PostFormValue("file")
		if file == "" {
			return badrequesterror.New("missing field: file")
		}

		if !filepath.IsAbs(file) {
			return badrequesterror.New("only absolute file paths are supported: %q", html.EscapeString(file))
		}

		task.File = &file
	}

	{
		uriRaw := r.PostFormValue("uri")
		if uriRaw == "" {
			return badrequesterror.New("missing field: uri")
		}

		uri, err := url.Parse(uriRaw)
		if err != nil {
			return badrequesterror.New("unable to parse URI: %q", html.EscapeString(uriRaw))
		}

		if uri.Scheme != "s3" {
			return badrequesterror.New("only s3 scheme is supported: %q", html.EscapeString(uriRaw))
		}

		bucket := uri.Host
		if bucket == "" {
			return badrequesterror.New("unable to parse bucket from URI: %q", html.EscapeString(uriRaw))
		}

		key := uri.Path[1:] // Remove leading slash

		task.Uri = &RequestURL{*uri}
		task.Bucket = &bucket
		task.Key = &key
	}

	{
		// the slug param is optional
		slug := r.PostFormValue("slug")
		if slug != "" {
			task.Slug = &slug
		}
	}

	// obtain file size to determine the number of upload parts and to compute
	// the transfer rate later
	fStat, err := os.Stat(*task.File)
	if err != nil {
		return badrequesterror.Wrap(err, "could not stat file: %q", *task.File)
	}
	task.SizeBytes = fStat.Size()
	// if the file is empty, we still need to upload it, so set the part size to 1
	task.UploadParts = max(divCeil(task.SizeBytes, h.conf.UploadPartsize.Value()), 1)

	return nil
}

func (h *S3ndHandler) uploadFileMultipart(ctx context.Context, task *UploadTask) error {
	file, err := os.Open(*task.File)
	if err != nil {
		return gherrors.Wrapf(err, "Could not open file %v to upload", *task.File)
	}
	defer file.Close()

	maxAttempts := *h.conf.UploadTries
attempts:
	for task.Attempts = 1; task.Attempts <= maxAttempts; task.Attempts++ {
		uploadAttempts.Inc()
		if task.Attempts > 1 {
			uploadRetries.Inc()
		}
		uploadCtx, cancel := context.WithTimeoutCause(ctx, *h.conf.UploadTimeout, errUploadAttemptTimeout)
		_, err = h.uploader.Upload(uploadCtx, &s3.PutObjectInput{
			Bucket: aws.String(*task.Bucket),
			Key:    aws.String(*task.Key),
			Body:   file,
		}, func(u *manager.Uploader) {
			u.Concurrency = int(task.UploadParts) // 1 go routine per upload part
		})
		cancel()

		var apiErr smithy.APIError
		var errMsg string

		switch {
		case err == nil: // upload succeeded
			break attempts
		case gherrors.As(err, &apiErr):
			// fail immediately if the bucket does not exist.  Retry all other s3 errors.
			if apiErr.ErrorCode() == "NoSuchBucket" {
				return errors.Join(errUploadNoSuchBucket, err)
			}
			errMsg = "s3 error"
		case gherrors.Is(context.Cause(uploadCtx), errUploadAttemptTimeout):
			errMsg = "timeout"
			err = errors.Join(errUploadAttemptTimeout, err)
		case gherrors.Is(err, context.Canceled): // the client disconnected
			return errors.Join(errUploadAborted, err)
		default: // unknown error -- could be a server side
			errMsg = "unknown error"
		}

		errMsg = fmt.Sprintf("%s during upload attempt %v/%v", errMsg, task.Attempts, maxAttempts)

		// bubble up the error if we've exhausted our attempts
		if task.Attempts == maxAttempts {
			return gherrors.Wrap(err, errMsg)
		}
		// otherwise, log the timeout and carry on
		logger.Warn(errMsg, slog.Any("task", task))
	}

	return nil
}

// based on the number of active uploads, adjust the packet pacing rate on all
// net.Conn's in the connection pool
func (h *S3ndHandler) updatePacingRate() {
	defer h.logUploads() // always log the current state

	var targetPace uint64
	bwLimitBytes := divCeil(h.conf.UploadBwlimit.Value(), 8)

	if bwLimitBytes == 0 {
		// noop if there is no, or effectively no, upload bandwidth limit configured
		return
	}

	// avoid div by zero
	if h.parallelUploads.GetCount() == 0 {
		targetPace = uint64(bwLimitBytes) //gosec:disable G115
	} else {
		targetPace = uint64(divCeil(bwLimitBytes, int64(h.parallelUploads.GetCount()))) //gosec:disable G115
	}

	if err := h.connTracker.SetPacingRate(targetPace); err != nil {
		logger.Error("unable to set pacing rate", slog.Any("error", err))
		return
	}
}

func (h *S3ndHandler) logUploads() {
	if h.conf.UploadBwlimit.Value() == 0 {
		// omit pacing rate logging if there is no upload bandwidth limit configured
		logger.Info(
			"active uploads",
			"uploads", h.parallelUploads.GetCount(),
			"queued", h.parallelUploads.Waiters(),
		)
	} else {
		logger.Info(
			"active uploads",
			"uploads", h.parallelUploads.GetCount(),
			"pace_mbits", h.connTracker.PacingRateMbits(),
			"pace_bytes", h.connTracker.PacingRate(),
			"queued", h.parallelUploads.Waiters(),
		)
	}
}

func divCeil(a, b int64) int64 {
	if b == 0 {
		panic("division by zero")
	}
	return (a + b - 1) / b // rounds up
}
