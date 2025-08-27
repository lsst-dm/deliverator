package util_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/lsst-dm/deliverator/util"
)

var _ = Describe("DivCeil", func() {
	It("should divide evenly when there is no remainder", func() {
		Expect(util.DivCeil(1, 1)).To(Equal(int64(1)))
		Expect(util.DivCeil(2, 2)).To(Equal(int64(1)))
	})

	It("should always round up", func() {
		Expect(util.DivCeil(1, 4)).To(Equal(int64(1)))
		Expect(util.DivCeil(1, 3)).To(Equal(int64(1)))
		Expect(util.DivCeil(1, 2)).To(Equal(int64(1)))
		Expect(util.DivCeil(5, 3)).To(Equal(int64(2)))
	})
})
