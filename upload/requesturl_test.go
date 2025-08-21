package upload_test

import (
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/lsst-dm/deliverator/upload"
)

var _ = Describe("RequestURL", func() {
	uri := "https://example.com/upload/upload-id"
	parsed, err := url.Parse(uri)
	Expect(err).NotTo(HaveOccurred())

	Context("when marshalling to JSON", func() {
		It("should return the original uri as a byte string", func() {
			rURL := &upload.RequestURL{
				URL: *parsed,
			}

			txt, err := rURL.MarshalText()
			Expect(err).NotTo(HaveOccurred())

			Expect(txt).To(Equal([]byte(uri)))
		})
	})

	Context("when unmarshalling from JSON", func() {
		It("should create a valid URL field", func() {
			rURL := &upload.RequestURL{}

			err := rURL.UnmarshalText([]byte(uri))
			Expect(err).NotTo(HaveOccurred())

			Expect(rURL.URL).To(Equal(*parsed))
		})
	})
})
