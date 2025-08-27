package s3nd_test

import (
	"io"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GET /metrics", func() {
	It("returns 200", func() {
		resp, err := http.Get(s3ndUrl.String() + "/metrics")
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		body, _ := io.ReadAll(resp.Body)
		Expect(string(body)).To(ContainSubstring("s3nd_upload_valid_requests_total"))
		Expect(string(body)).To(ContainSubstring("s3nd_upload_parts_active"))
		Expect(string(body)).To(ContainSubstring("s3nd_s3_tcp_conn_pace_max_bytes"))
	})
})
