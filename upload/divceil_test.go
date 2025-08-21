package upload

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("divCeil", func() {
	It("should divide when there is no modulus", func() {
		Expect(divCeil(1, 1)).To(Equal(int64(1)))
		Expect(divCeil(2, 2)).To(Equal(int64(1)))
	})

	It("should always round up", func() {
		Expect(divCeil(1, 4)).To(Equal(int64(1)))
		Expect(divCeil(1, 3)).To(Equal(int64(1)))
		Expect(divCeil(1, 2)).To(Equal(int64(1)))
		Expect(divCeil(5, 3)).To(Equal(int64(2)))
	})
})
