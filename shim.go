package tinfoilconfig

import (
	"fmt"
	"slices"
	"time"

	"github.com/creasty/defaults"
	"gopkg.in/yaml.v3"
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

func DecodeShim(node *yaml.Node) (*ShimConfig, error) {
	if err := validateYAMLTree(node); err != nil {
		return nil, fmt.Errorf("failed to decode config: %v", err)
	}
	var config ShimConfig
	if err := defaults.Set(&config); err != nil {
		return nil, fmt.Errorf("failed to set defaults: %v", err)
	}
	if err := node.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to decode config: %v", err)
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &config, nil
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

func validateYAMLTree(root *yaml.Node) error {
	if root == nil {
		return fmt.Errorf("missing YAML document")
	}
	type pendingNode struct {
		node  *yaml.Node
		depth int
	}
	stack := []pendingNode{{node: root, depth: 1}}
	const maxNodes = 16384
	const maxDepth = 64
	nodes := 0
	valueBytes := 0
	for len(stack) > 0 {
		pending := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if pending.node == nil {
			continue
		}
		nodes++
		if nodes > maxNodes {
			return fmt.Errorf("YAML node count exceeds %d", maxNodes)
		}
		if pending.depth > maxDepth {
			return fmt.Errorf("YAML nesting exceeds depth %d", maxDepth)
		}
		if pending.node.Kind == yaml.AliasNode || pending.node.Alias != nil {
			return fmt.Errorf("YAML aliases are unsupported")
		}
		valueBytes += len(pending.node.Value) + len(pending.node.Tag) + len(pending.node.Anchor)
		if valueBytes > 1<<20 {
			return fmt.Errorf("YAML scalar data exceeds %d-byte limit", 1<<20)
		}
		for index := len(pending.node.Content) - 1; index >= 0; index-- {
			stack = append(stack, pendingNode{node: pending.node.Content[index], depth: pending.depth + 1})
		}
	}
	return nil
}
