package tinfoilconfig

import (
	"fmt"
	"slices"

	"github.com/creasty/defaults"
	"gopkg.in/yaml.v3"
)

type ShimConfig struct {
	ApplicationPolicy string   `yaml:"application-policy,omitempty"`
	UpstreamPort      int      `yaml:"upstream-port"`
	UpstreamContainer string   `yaml:"upstream-container,omitempty"`
	Paths             []string `yaml:"paths,omitempty"`
	OriginDomains     []string `yaml:"origins,omitempty"`

	TLSMode          string `yaml:"tls-mode" default:"cert-proxy"`
	TLSEnv           string `yaml:"tls-env" default:"production"`
	TLSChallengeMode string `yaml:"tls-challenge" default:"dns"`
	TLSWildcard      bool   `yaml:"tls-wildcard" default:"false"`
	TLSOwnSANDomain  bool   `yaml:"tls-own-san-domain" default:"false"`

	ControlPlane           string    `yaml:"control-plane" default:"https://api.tinfoil.sh"`
	ATC                    string    `yaml:"atc" default:"https://atc.tinfoil.sh"`
	Authenticated          bool      `yaml:"authenticated,omitempty" default:"false"`
	AuthenticatedEndpoints *[]string `yaml:"authenticated-endpoints,omitempty"`
	RateLimit              float64   `yaml:"rate-limit,omitempty"`
	RateBurst              int       `yaml:"rate-burst,omitempty"`
	Email                  string    `yaml:"email" default:"tls@tinfoil.sh"`
	PublishAttestation     bool      `yaml:"publish-attestation" default:"true"`
	DummyAttestation       bool      `yaml:"dummy-attestation" default:"false"`
	ExpectedGPUs           int       `yaml:"expected-gpus" default:"0"`
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
	if c.ApplicationPolicy != "" && c.ApplicationPolicy != "workload" {
		return fmt.Errorf("application-policy must be workload when set")
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

// ValidateWorkloadShim rejects even zero-valued legacy settings, so an image
// switch cannot silently discard application authentication or path policy.
func ValidateWorkloadShim(node *yaml.Node) error {
	if node == nil || node.Kind != yaml.MappingNode || len(node.Content)%2 != 0 {
		return fmt.Errorf("workload policy requires a shim mapping")
	}
	owner := ""
	for i := 0; i < len(node.Content); i += 2 {
		field := node.Content[i].Value
		switch field {
		case "authenticated", "authenticated-endpoints", "paths", "origins", "rate-limit", "rate-burst":
			return fmt.Errorf("shim.%s is unsupported by workload-policy images; move it into the workload", field)
		case "application-policy":
			owner = node.Content[i+1].Value
		}
	}
	if owner != "workload" {
		return fmt.Errorf("this image requires shim.application-policy: workload")
	}
	return nil
}
