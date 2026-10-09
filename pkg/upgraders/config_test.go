package upgraders

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("scaleConfig", func() {
	Describe("IsValid", func() {
		It("returns no error for valid config with no extra machine pools", func() {
			cfg := &scaleConfig{TimeOut: 30}
			Expect(cfg.IsValid()).NotTo(HaveOccurred())
		})

		It("returns no error for valid config with valid extra machine pool patterns", func() {
			cfg := &scaleConfig{
				TimeOut:           30,
				ExtraMachinePools: []string{"non-serving-*", "infra-*"},
			}
			Expect(cfg.IsValid()).NotTo(HaveOccurred())
		})

		It("returns an error when TimeOut is zero", func() {
			cfg := &scaleConfig{TimeOut: 0}
			err := cfg.IsValid()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("config scale timeOut is invalid"))
		})

		It("returns an error when TimeOut is negative", func() {
			cfg := &scaleConfig{TimeOut: -1}
			err := cfg.IsValid()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("config scale timeOut is invalid"))
		})

		It("returns an error for a malformed glob pattern", func() {
			cfg := &scaleConfig{
				TimeOut:           30,
				ExtraMachinePools: []string{"[invalid"},
			}
			err := cfg.IsValid()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("config scale extraMachinePools contains invalid pattern"))
			Expect(err.Error()).To(ContainSubstring("[invalid"))
		})

		It("returns an error when one pattern among many is malformed", func() {
			cfg := &scaleConfig{
				TimeOut:           30,
				ExtraMachinePools: []string{"non-serving-*", "[bad", "infra-*"},
			}
			err := cfg.IsValid()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("[bad"))
		})

		It("returns no error for an empty extra machine pools slice", func() {
			cfg := &scaleConfig{
				TimeOut:           30,
				ExtraMachinePools: []string{},
			}
			Expect(cfg.IsValid()).NotTo(HaveOccurred())
		})

		It("accepts exact pool names without wildcards", func() {
			cfg := &scaleConfig{
				TimeOut:           30,
				ExtraMachinePools: []string{"non-serving-9a"},
			}
			Expect(cfg.IsValid()).NotTo(HaveOccurred())
		})

		It("accepts patterns with character classes", func() {
			cfg := &scaleConfig{
				TimeOut:           30,
				ExtraMachinePools: []string{"non-serving-[0-9]*"},
			}
			Expect(cfg.IsValid()).NotTo(HaveOccurred())
		})

		It("accepts patterns with question mark wildcards", func() {
			cfg := &scaleConfig{
				TimeOut:           30,
				ExtraMachinePools: []string{"non-serving-?a"},
			}
			Expect(cfg.IsValid()).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("upgraderConfig", func() {
	Describe("IsValid", func() {
		var cfg *upgraderConfig
		BeforeEach(func() {
			cfg = buildTestUpgraderConfig(90, 30, 8, 120, 30)
			cfg.NodeDrain.Timeout = 45
		})

		It("returns an error when scale config has an invalid pattern", func() {
			cfg.Scale.ExtraMachinePools = []string{"[invalid"}
			err := cfg.IsValid()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("config scale extraMachinePools contains invalid pattern"))
		})

		It("passes validation when scale config has valid patterns", func() {
			cfg.Scale.ExtraMachinePools = []string{"non-serving-*"}
			Expect(cfg.IsValid()).NotTo(HaveOccurred())
		})

		It("passes validation when scale config has no extra machine pools", func() {
			Expect(cfg.IsValid()).NotTo(HaveOccurred())
		})
	})
})
