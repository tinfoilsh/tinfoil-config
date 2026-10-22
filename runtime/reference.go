package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"golang.org/x/mod/semver"
)

const (
	VariantCPU = "cpu"
	VariantGPU = "gpu"
)

type Reference struct {
	Tag     string
	Variant string
	Digest  string
}

func ParseVersion(value string) (tag, variant string, err error) {
	version := "v" + strings.TrimPrefix(value, "v")
	tag = semver.Canonical(version)
	metadata := semver.Build(version)
	if tag == "" || tag+metadata != version {
		return "", "", fmt.Errorf("runtime version must be a full semantic version")
	}
	variant = strings.TrimPrefix(metadata, "+")
	if variant != "" && variant != VariantGPU {
		return "", "", fmt.Errorf("unsupported runtime variant %q", variant)
	}
	return tag, variant, nil
}

func ParseReference(value string) (Reference, error) {
	version, digest, pinned := strings.Cut(value, "@sha256:")
	tag, variant, err := ParseVersion(version)
	if err != nil {
		return Reference{}, err
	}
	decoded, err := hex.DecodeString(digest)
	if !pinned || err != nil || len(decoded) != sha256.Size || digest != strings.ToLower(digest) {
		return Reference{}, fmt.Errorf("runtime requires <version>@sha256:<lowercase manifest digest>")
	}
	return Reference{Tag: tag, Variant: variant, Digest: digest}, nil
}

func (r Reference) ImageVariant() string {
	if r.Variant == "" {
		return VariantCPU
	}
	return r.Variant
}
