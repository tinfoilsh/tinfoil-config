package tinfoilconfig

import (
	"fmt"
	"slices"
	"time"
)

const DefaultDrainTimeout = 5 * time.Minute

type ShimConfig struct {
	UpstreamPort      int      `yaml:"upstream-port" json:"upstream-port"`
	UpstreamContainer string   `yaml:"upstream-container,omitempty" json:"upstream-container"`
	Paths             []string `yaml:"paths" json:"paths"`
	OriginDomains     []string `yaml:"origins" json:"origins"`

	TLSMode          string `yaml:"tls-mode" default:"cert-proxy" json:"tls-mode"`
	TLSEnv           string `yaml:"tls-env" default:"production" json:"tls-env"`
	TLSChallengeMode string `yaml:"tls-challenge" default:"dns" json:"tls-challenge"`
	TLSWildcard      bool   `yaml:"tls-wildcard" default:"false" json:"tls-wildcard"`
	TLSOwnSANDomain  bool   `yaml:"tls-own-san-domain" default:"false" json:"tls-own-san-domain"`

	ControlPlane           string        `yaml:"control-plane" default:"https://api.tinfoil.sh" json:"control-plane"`
	ATC                    string        `yaml:"atc" default:"https://atc.tinfoil.sh" json:"atc"`
	Authenticated          bool          `yaml:"authenticated" default:"false" json:"authenticated"`
	AuthenticatedEndpoints *[]string     `yaml:"authenticated-endpoints" json:"authenticated-endpoints"`
	RateLimit              float64       `yaml:"rate-limit" json:"rate-limit"`
	RateBurst              int           `yaml:"rate-burst" json:"rate-burst"`
	DrainConnections       bool          `yaml:"drain_connections" default:"false" json:"drain_connections"`
	DrainTimeout           time.Duration `yaml:"drain_timeout" json:"drain_timeout"`
	Email                  string        `yaml:"email" default:"tls@tinfoil.sh" json:"email"`
	PublishAttestation     bool          `yaml:"publish-attestation" default:"true" json:"publish-attestation"`
	DummyAttestation       bool          `yaml:"dummy-attestation" default:"false" json:"dummy-attestation"`
	ExpectedGPUs           int           `yaml:"expected-gpus" default:"0" json:"expected-gpus"`
}

func (c *ShimConfig) SetDefaults() {
	if c.DrainTimeout == 0 {
		c.DrainTimeout = DefaultDrainTimeout
	}
}

func (c *ShimConfig) Validate() error {
	if c.DrainTimeout < 0 || c.DrainConnections && c.DrainTimeout == 0 {
		return fmt.Errorf("drain_timeout must be positive")
	}
	if c.UpstreamPort == 0 {
		return fmt.Errorf("upstream port is not set")
	}
	if !slices.Contains([]string{"self-signed", "acme", "cert-proxy"}, c.TLSMode) {
		return fmt.Errorf("invalid TLS mode: %s (must be self-signed, acme, or cert-proxy)", c.TLSMode)
	}
	if !slices.Contains([]string{"production", "staging"}, c.TLSEnv) {
		return fmt.Errorf("invalid TLS environment: %s (must be production or staging)", c.TLSEnv)
	}
	if !slices.Contains([]string{"tls", "dns", "http"}, c.TLSChallengeMode) {
		return fmt.Errorf("invalid TLS challenge mode: %s (must be tls, dns, or http)", c.TLSChallengeMode)
	}
	if c.TLSWildcard && c.TLSChallengeMode != "dns" {
		return fmt.Errorf("tls-wildcard requires tls-challenge: dns (wildcard certs cannot use %s challenge)", c.TLSChallengeMode)
	}
	return nil
}
