package conf_test

import (
	"flag"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/lsst-dm/deliverator/v2/conf"

	k8sresource "k8s.io/apimachinery/pkg/api/resource"
)

var _ = Describe("S3ndConf", func() {
	Describe("UploadBwlimitBytes()", func() {
		It("should return bytes instead of bits", func() {
			limit, _ := k8sresource.ParseQuantity("1Ki")
			conf := conf.S3ndConf{
				UploadBwlimit: &limit,
			}

			Expect(conf.UploadBwlimitBytes()).To(Equal(int64(128)))
		})
	})

	Describe("NewConf()", func() {
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

		Describe("S3ND_UPLOAD_TIMEOUT_FACTOR", func() {
			It("should default to 1", func() {
				conf := conf.NewS3ndConf("1.2.3")

				Expect(conf.UploadTimeoutFactor()).To(Equal(1))
			})

			It("should read from env", func() {
				err := os.Setenv("S3ND_UPLOAD_TIMEOUT_FACTOR", "2")
				Expect(err).ToNot(HaveOccurred())

				conf := conf.NewS3ndConf("1.2.3")

				Expect(conf.UploadTimeoutFactor()).To(Equal(2))
			})
		})
	})
})
