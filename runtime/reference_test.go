package runtime

import (
	"strings"
	"testing"
)

func TestParseReference(t *testing.T) {
	digest := strings.Repeat("a", 64)
	for _, test := range []struct {
		version string
		tag     string
		variant string
		image   string
	}{
		{"0.15.0", "v0.15.0", "", VariantCPU},
		{"v0.15.0+gpu", "v0.15.0", VariantGPU, VariantGPU},
		{"0.15.1-rc.12+gpu", "v0.15.1-rc.12", VariantGPU, VariantGPU},
	} {
		t.Run(test.version, func(t *testing.T) {
			ref, err := ParseReference(test.version + "@sha256:" + digest)
			if err != nil {
				t.Fatal(err)
			}
			if ref != (Reference{Tag: test.tag, Variant: test.variant, Digest: digest}) || ref.ImageVariant() != test.image {
				t.Fatalf("reference = %#v, image = %q", ref, ref.ImageVariant())
			}
		})
	}
	for _, value := range []string{
		"0.15@sha256:" + digest,
		"0.15.0+cpu@sha256:" + digest,
		"0.15.0+initrd@sha256:" + digest,
		"0.15.0+gpu.extra@sha256:" + digest,
		"0.15.0+GPU@sha256:" + digest,
		"0.15.0+gpu",
		"0.15.0@sha256:" + strings.ToUpper(digest),
		"0.15.0@sha256:" + digest[:62],
		"0.15.0@sha256:" + digest + "@sha256:" + digest,
	} {
		if ref, err := ParseReference(value); err == nil {
			t.Errorf("accepted %q as %#v", value, ref)
		}
	}
}
