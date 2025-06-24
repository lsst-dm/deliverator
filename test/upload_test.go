package s3nd_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	minio "github.com/minio/minio-go/v7"
)

var _ = Describe("POST /upload", func() {
	It("returns 200", func() {
		f, err := os.CreateTemp("", "test1.txt")
		Expect(err).NotTo(HaveOccurred())
		defer os.Remove(f.Name())
		defer f.Close()
		_, err = f.WriteString("test1 test1 test1\n")
		Expect(err).NotTo(HaveOccurred())

		resp, err := http.PostForm(s3ndUrl+"/upload",
			url.Values{"file": {f.Name()}, "uri": {"s3://" + s3ndBucket + "/foo"}})
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	It("created an s3 object", func() {
		o, err := s3.GetObject(context.Background(), s3ndBucket, "foo", minio.GetObjectOptions{})
		Expect(err).NotTo(HaveOccurred())
		defer o.Close()

		data, err := io.ReadAll(o)
		Expect(err).NotTo(HaveOccurred())

		Expect(string(data)).To(Equal("test1 test1 test1\n"))
	})
})
