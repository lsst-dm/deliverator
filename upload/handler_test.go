package upload_test

import (
	"flag"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/lsst-dm/deliverator/v2/conf"
	"github.com/lsst-dm/deliverator/v2/upload"
)

var _ = Describe("S3ndHandler", func() {
	BeforeEach(func() {
		os.Clearenv()

		// required env var for NewConf to succeed
		err := os.Setenv("S3ND_ENDPOINT_URL", "http://example.com")
		Expect(err).ToNot(HaveOccurred())

		// prevent testing flags from being visible, E.g. `-test.timeout`
		os.Args = []string{"s3nd"}

		// reset flag set to prevent flag redefined panic
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	})
	AfterEach(func() {
		os.Clearenv()
	})

	Describe("uploadTimeout()", func() {
		Context("when factor is 1", func() {
			BeforeEach(func() {
				err := os.Setenv("S3ND_UPLOAD_TIMEOUT", "5s")
				Expect(err).ToNot(HaveOccurred())
				err = os.Setenv("S3ND_UPLOAD_TIMEOUT_FACTOR", "1")
				Expect(err).ToNot(HaveOccurred())
			})

			Context("1st upload attempt", func() {
				It("should return 1x", func() {
					h := upload.NewS3ndHandler(conf.NewS3ndConf("1.2.3"))
					Expect(h.UploadTimeout(1)).To(Equal(time.Duration(5 * time.Second)))
				})
			})

			Context("2nd upload attempt", func() {
				It("should return 1x", func() {
					h := upload.NewS3ndHandler(conf.NewS3ndConf("1.2.3"))
					Expect(h.UploadTimeout(2)).To(Equal(time.Duration(5 * time.Second)))
				})
			})

			Context("3rd upload attempt", func() {
				It("should return 1x", func() {
					h := upload.NewS3ndHandler(conf.NewS3ndConf("1.2.3"))
					Expect(h.UploadTimeout(3)).To(Equal(time.Duration(5 * time.Second)))
				})
			})
		})

		Context("when factor is 2", func() {
			BeforeEach(func() {
				err := os.Setenv("S3ND_UPLOAD_TIMEOUT", "5s")
				Expect(err).ToNot(HaveOccurred())
				err = os.Setenv("S3ND_UPLOAD_TIMEOUT_FACTOR", "2")
				Expect(err).ToNot(HaveOccurred())
			})

			Context("1st upload attempt", func() {
				It("should return 1x", func() {
					h := upload.NewS3ndHandler(conf.NewS3ndConf("1.2.3"))
					Expect(h.UploadTimeout(1)).To(Equal(time.Duration(5 * time.Second)))
				})
			})

			Context("2nd upload attempt", func() {
				It("should return 2x", func() {
					h := upload.NewS3ndHandler(conf.NewS3ndConf("1.2.3"))
					Expect(h.UploadTimeout(2)).To(Equal(time.Duration(10 * time.Second)))
				})
			})

			Context("3rd upload attempt", func() {
				It("should return 4x", func() {
					h := upload.NewS3ndHandler(conf.NewS3ndConf("1.2.3"))
					Expect(h.UploadTimeout(3)).To(Equal(time.Duration(20 * time.Second)))
				})
			})
		})
	})
})
