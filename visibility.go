package tinfoilconfig

import (
	"strings"

	"golang.org/x/mod/semver"
)

const (
	PrivateConfigMinVersion = "v0.15.3-rc.1"
	VisibilityPublic        = "public"
	VisibilityPrivate       = "private"
)

func (c *Config) DigestOnlyAttestation() bool {
	version, _, _ := strings.Cut(c.CVMVersion, "@sha256:")
	return semver.Compare("v"+strings.TrimPrefix(version, "v"), PrivateConfigMinVersion) >= 0
}

func (c *Config) IsPublic() bool {
	return c.Security.ConfigVisibility == VisibilityPublic || (c.Security.ConfigVisibility == "" && !c.DigestOnlyAttestation())
}
