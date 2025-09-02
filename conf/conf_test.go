package conf_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/lsst-dm/deliverator/v2/conf"

	k8sresource "k8s.io/apimachinery/pkg/api/resource"
)

var _ = Describe("Conf", func() {
	Describe("UploadBwlimitBytes()", func() {
		It("should return bytes instead of bits", func() {
			limit, _ := k8sresource.ParseQuantity("1Ki")
			conf := conf.S3ndConf{
				UploadBwlimit: &limit,
			}

			Expect(conf.UploadBwlimitBytes()).To(Equal(int64(128)))
		})
	})
})
