package s3nd_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/lsst-dm/deliverator/client"
	"github.com/lsst-dm/deliverator/upload"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GET /metrics", func() {
	BeforeEach(func() {
		// Make a request to ensure we have some metrics.
		// In particular we want to ensure we have some histogram metrics.
		var status *upload.RequestStatus
		var uri url.URL
		var file string
		slug := "banana"
		testFile := "test2"

		f, err := os.CreateTemp("", testFile)
		Expect(err).NotTo(HaveOccurred())
		defer os.Remove(f.Name())
		defer f.Close()
		for i := 0; i < 3; i++ {
			_, err = f.WriteString(testFile + "\n")
			Expect(err).NotTo(HaveOccurred())
		}
		_ = f.Sync()

		s3nd := client.NewClient(s3ndUrl)
		file = f.Name()
		uri = url.URL{
			Scheme: "s3",
			Host:   s3ndBucket,
			Path:   "/" + testFile,
		}

		status, err = s3nd.Upload(context.Background(), file, uri, slug)
		Expect(err).NotTo(HaveOccurred())
		Expect(status.Code).To(Equal(http.StatusOK))
	})

	It("returns 200", func() {
		resp, err := http.Get(s3ndUrl.String() + "/metrics")
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		body, _ := io.ReadAll(resp.Body)

		Expect(string(body)).To(ContainSubstring("s3nd_upload_valid_requests_total"))
		Expect(string(body)).To(ContainSubstring("s3nd_upload_parts_active"))
		Expect(string(body)).To(ContainSubstring("s3nd_s3_tcp_conn_pace_max_bytes"))

		// Check for histogram metrics
		// note that these are native Prometheus histogram metrics which use protobufs
		// The "classic" text format is being checked here for convenience
		histogramMetrics := []string{
			"s3nd_upload_total_seconds",
			"s3nd_upload_queued_seconds",
			"s3nd_upload_transfer_seconds",
			"s3nd_upload_transfer_rate_bytes",
			"s3nd_upload_transfer_size_bytes",
		}

		for _, m := range histogramMetrics {
			Expect(string(body)).To(ContainSubstring(m + "_count"))
			Expect(string(body)).To(ContainSubstring(m + "_sum"))
			Expect(string(body)).To(ContainSubstring(m + "_bucket"))
		}
	})
})
