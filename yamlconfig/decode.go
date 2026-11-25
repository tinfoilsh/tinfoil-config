// Package yamlconfig reads the YAML form of a Tinfoil config. YAML is what
// cvm-versions before tinfoilconfig.JSONConfigMinVersion measure, and what
// people write for every version; from that version on the measured config is
// JSON, which this package produces with ToJSON and Normalize.
//
// Programs that meet YAML import this package: a registry, a command-line
// tool, a release action. A program that only reads measured configs of
// current versions, such as the guest, imports the root package alone and
// links no YAML parser through this module.
package yamlconfig

import (
	"bytes"
	"encoding/json/v2"
	"fmt"
	"io"
	"regexp"
	"slices"

	"github.com/creasty/defaults"
	tinfoilconfig "github.com/tinfoilsh/tinfoil-config"
	"gopkg.in/yaml.v3"
)

const (
	yamlNullTag = "!!null"
	yamlIntTag  = "!!int"
	// mergeKey is the YAML key that merges another mapping into this one.
	mergeKey = "<<"
)

var repositoryIDPattern = regexp.MustCompile(`^[1-9][0-9]{0,17}$`)

// Decode reads and validates a config of any version: JSON when it pins
// tinfoilconfig.JSONConfigMinVersion or a later cvm-version, YAML otherwise.
// YAML for a version that takes JSON is an error; translate it with ToJSON.
func Decode(data []byte, options tinfoilconfig.Options) (*tinfoilconfig.Config, error) {
	if len(data) > tinfoilconfig.MaxConfigBytes {
		return nil, fmt.Errorf("parsing config: input exceeds %d-byte limit", tinfoilconfig.MaxConfigBytes)
	}
	if version, ok := jsonCVMVersion(data); ok && tinfoilconfig.RequiresJSON(version) {
		return tinfoilconfig.Decode(data, options)
	}
	config, err := decodeYAML(data, options)
	if err != nil {
		return nil, err
	}
	if tinfoilconfig.RequiresJSON(config.CVMVersion) {
		return nil, fmt.Errorf("parsing config: cvm-version %s and later take a JSON config; convert this one with ToJSON", tinfoilconfig.JSONConfigMinVersion)
	}
	return config, nil
}

// ValidateBytes reports whether Decode accepts data.
func ValidateBytes(data []byte, options tinfoilconfig.Options) error {
	_, err := Decode(data, options)
	return err
}

// jsonCVMVersion returns the cvm-version of data read as a JSON object, and
// whether data is one that states it.
func jsonCVMVersion(data []byte) (string, bool) {
	var pin struct {
		CVMVersion *string `json:"cvm-version"`
	}
	if err := json.Unmarshal(data, &pin); err != nil || pin.CVMVersion == nil {
		return "", false
	}
	return *pin.CVMVersion, true
}

// yamlConfig is the top level of a YAML config. Its members are those of
// tinfoilconfig.Config, with the types whose YAML form has rules of its own.
type yamlConfig struct {
	CVMVersion   string                         `yaml:"cvm-version"`
	Shim         yaml.Node                      `yaml:"shim"`
	CVMNetwork   tinfoilconfig.CVMNetworkConfig `yaml:"cvm-network"`
	Networks     map[string]*networkSpec        `yaml:"networks"`
	CPUs         int                            `yaml:"cpus"`
	Memory       int                            `yaml:"memory"`
	GPUs         int                            `yaml:"gpus"`
	Security     tinfoilconfig.SecurityConfig   `yaml:"security,omitempty"`
	Models       []tinfoilconfig.ModelSpec      `yaml:"models"`
	Volumes      []tinfoilconfig.VolumeSpec     `yaml:"volumes"`
	AttestedKeys []tinfoilconfig.AttestedKey    `yaml:"attested-keys,omitempty"`
	Containers   []container                    `yaml:"containers"`
	KeyserverURL string                         `yaml:"keyserver-url,omitempty"`
}

func decodeYAML(data []byte, options tinfoilconfig.Options) (*tinfoilconfig.Config, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var read yamlConfig
	if err := decoder.Decode(&read); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("parsing config: multiple YAML documents")
		}
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	shimConfig, err := DecodeShim(&read.Shim)
	if err != nil {
		return nil, fmt.Errorf("parsing shim config: %w", err)
	}
	config := &tinfoilconfig.Config{
		CVMVersion: read.CVMVersion, CVMNetwork: read.CVMNetwork,
		CPUs: read.CPUs, Memory: read.Memory, GPUs: read.GPUs, Security: read.Security,
		Models: read.Models, Volumes: read.Volumes, AttestedKeys: read.AttestedKeys, KeyserverURL: read.KeyserverURL,
	}
	if read.Networks != nil {
		config.Networks = make(map[string]*tinfoilconfig.NetworkSpec, len(read.Networks))
		for name, network := range read.Networks {
			config.Networks[name] = (*tinfoilconfig.NetworkSpec)(network)
		}
	}
	if read.Containers != nil {
		config.Containers = make([]tinfoilconfig.Container, len(read.Containers))
	}
	for index, container := range read.Containers {
		config.Containers[index] = container.Container
		if container.unsupported != "" {
			return nil, fmt.Errorf("containers[%d].%s is unsupported", index, container.unsupported)
		}
	}
	if err := tinfoilconfig.Finalize(config, shimConfig, options); err != nil {
		return nil, err
	}
	return config, nil
}

// DecodeShim reads the shim settings of a YAML config.
func DecodeShim(node *yaml.Node) (*tinfoilconfig.ShimConfig, error) {
	if err := validateYAMLTree(node); err != nil {
		return nil, fmt.Errorf("failed to decode config: %v", err)
	}
	var config tinfoilconfig.ShimConfig
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

// networkSpec is a network as YAML states it: a mapping, or nothing at all
// for a network with no egress.
type networkSpec tinfoilconfig.NetworkSpec

func (n *networkSpec) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		if node.Tag != yamlNullTag {
			return fmt.Errorf("network entry must be a mapping or null")
		}
		n.Egress = "closed"
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("network entry must be a mapping")
	}
	seen := map[string]bool{}
	for index := 0; index < len(node.Content); index += 2 {
		field := node.Content[index].Value
		if seen[field] {
			return fmt.Errorf("duplicate network field %q", field)
		}
		seen[field] = true
		if field != "egress" && field != "allow" {
			return fmt.Errorf("unknown network field %q", field)
		}
	}
	var raw tinfoilconfig.NetworkSpec
	if err := node.Decode(&raw); err != nil {
		return err
	}
	*n = networkSpec(raw)
	if n.Egress == "" {
		n.Egress = "closed"
	}
	return nil
}

// container is a container as YAML states it. Docker Compose fields that the
// runtime does not support are known by name, so that a config using one is
// told so instead of failing as an unknown field.
type container struct {
	tinfoilconfig.Container
	unsupported string
}

var unsupportedContainerFields = []string{"privileged", "cap_drop", "security_opt"}

var containerFields = []string{
	"name", "image", "provenance", "cvm_admin", "seal_register", "attestation", "command", "entrypoint",
	"working_dir", "user", "env", "secrets", "models", "keys",
	"volumes", "persistent_volumes", "devices", "cap_add", "runtime",
	"networks", "ports", "ipc", "pid", "gpus",
	"shm_size", "memory", "cpus", "tmpfs",
	"read_only", "pids_limit", "restart",
	"stop_signal", "stop_timeout", "healthcheck",
}

func (c *container) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("container entry must be a mapping")
	}
	var unsupported string
	var provenance, healthcheck *yaml.Node
	seen := map[string]bool{}
	for index := 0; index < len(node.Content); index += 2 {
		field, value := node.Content[index].Value, resolved(node.Content[index+1])
		if field == mergeKey {
			return fmt.Errorf("container YAML merge keys are unsupported")
		}
		if seen[field] {
			return fmt.Errorf("duplicate container field %q", field)
		}
		seen[field] = true
		switch {
		case slices.Contains(unsupportedContainerFields, field):
			if unsupported == "" || slices.Index(unsupportedContainerFields, field) < slices.Index(unsupportedContainerFields, unsupported) {
				unsupported = field
			}
		case !slices.Contains(containerFields, field):
			return fmt.Errorf("unknown container field %q", field)
		case value.ShortTag() == yamlNullTag:
		case field == "provenance":
			provenance = value
		case field == "healthcheck":
			healthcheck = value
		}
	}
	if provenance != nil {
		if err := checkProvenance(provenance); err != nil {
			return err
		}
	}
	if healthcheck != nil {
		if err := checkHealthcheck(healthcheck); err != nil {
			return err
		}
	}
	var raw tinfoilconfig.Container
	if err := node.Decode(&raw); err != nil {
		return err
	}
	*c = container{Container: raw, unsupported: unsupported}
	return nil
}

// checkProvenance applies the rules a provenance section has in YAML before
// it is decoded: only known fields, none repeated, the repository ID written
// in plain decimal, and the commit a scalar read as the text written, even
// when that text looks like a number.
func checkProvenance(node *yaml.Node) error {
	if err := requireKnownFields(node, "provenance", "github"); err != nil {
		return err
	}
	for index := 0; index < len(node.Content); index += 2 {
		github := resolved(node.Content[index+1])
		if github.ShortTag() == yamlNullTag {
			continue
		}
		if err := requireKnownFields(github, "provenance.github", "repository", "repository_id", "workflow", "ref", "commit"); err != nil {
			return err
		}
		for member := 0; member < len(github.Content); member += 2 {
			value := resolved(github.Content[member+1])
			if github.Content[member].Value == "repository_id" && (value.Kind != yaml.ScalarNode || value.Tag != yamlIntTag || !repositoryIDPattern.MatchString(value.Value)) {
				return fmt.Errorf("provenance.github.repository_id must be the repository's numeric ID in plain decimal")
			}
			var commit string
			if github.Content[member].Value == "commit" && (value.Kind != yaml.ScalarNode || value.Decode(&commit) != nil || commit != value.Value) {
				return fmt.Errorf("provenance.github.commit must be the commit hash as text; quote it")
			}
		}
	}
	return nil
}

func checkHealthcheck(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("healthcheck must be a mapping")
	}
	allowed := []string{"test", "interval", "timeout", "retries", "start_period"}
	seen := map[string]bool{}
	for index := 0; index < len(node.Content); index += 2 {
		field := node.Content[index].Value
		if seen[field] {
			return fmt.Errorf("duplicate healthcheck field %q", field)
		}
		seen[field] = true
		if !slices.Contains(allowed, field) {
			return fmt.Errorf("unknown healthcheck field %q", field)
		}
	}
	return nil
}

func requireKnownFields(node *yaml.Node, name string, allowed ...string) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("%s must be a mapping", name)
	}
	seen := map[string]bool{}
	for index := 0; index < len(node.Content); index += 2 {
		field := node.Content[index].Value
		if seen[field] {
			return fmt.Errorf("duplicate %s field %q", name, field)
		}
		seen[field] = true
		if !slices.Contains(allowed, field) {
			return fmt.Errorf("unknown %s field %q", name, field)
		}
	}
	return nil
}

// resolved returns the node an alias stands for, or node itself.
func resolved(node *yaml.Node) *yaml.Node {
	if node.Kind == yaml.AliasNode && node.Alias != nil {
		return node.Alias
	}
	return node
}
