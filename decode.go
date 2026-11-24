package tinfoilconfig

import (
	"encoding/json/jsontext"
	"fmt"
)

const MaxConfigBytes = 1 << 20

// Decode reads and validates a config. A config is JSON in its one byte form
// (see CheckJSONForm) and pins JSONConfigMinVersion or a later cvm-version.
// This package reads nothing else: the YAML that earlier versions measure,
// and that people write, is read and translated by the yamlconfig package.
func Decode(data []byte, options Options) (*Config, error) {
	if len(data) > MaxConfigBytes {
		return nil, fmt.Errorf("parsing config: input exceeds %d-byte limit", MaxConfigBytes)
	}
	if err := CheckJSONForm(data); err != nil {
		return nil, fmt.Errorf("parsing config: cvm-version %s and later take a JSON config: %w", JSONConfigMinVersion, err)
	}
	var input struct {
		*Config
		Shim jsontext.Value `json:"shim"`
	}
	input.Config = &Config{}
	if err := unmarshalJSON(data, &input); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	config := input.Config
	if !RequiresJSON(config.CVMVersion) {
		return nil, fmt.Errorf("parsing config: a JSON config pins cvm-version %s or later (got %q)", JSONConfigMinVersion, config.CVMVersion)
	}
	shimConfig, err := decodeShimJSON(input.Shim)
	if err != nil {
		return nil, fmt.Errorf("parsing shim config: %w", err)
	}
	for index := range config.Containers {
		for _, item := range config.Containers[index].Env {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			for name, value := range entry {
				if _, ok := value.(string); !ok {
					return nil, fmt.Errorf("parsing config: containers[%d].env.%s must be a string", index, name)
				}
			}
		}
	}
	if err := Finalize(config, shimConfig, options); err != nil {
		return nil, err
	}
	return config, nil
}

// Finalize attaches the shim settings to a decoded config, fills the values
// that depend on the whole config, and validates it. A decoder calls it last.
func Finalize(config *Config, shimConfig *ShimConfig, options Options) error {
	shimConfig.ExpectedGPUs = config.GPUs
	if shimConfig.UpstreamContainer == "" && len(config.Containers) > 0 {
		shimConfig.UpstreamContainer = config.Containers[0].Name
	}
	config.ShimCfg = shimConfig
	setNetworkDefaults(config)
	return Validate(config, options)
}

func ValidateBytes(data []byte, options Options) error {
	_, err := Decode(data, options)
	return err
}

func setNetworkDefaults(config *Config) {
	for name, spec := range config.Networks {
		if spec == nil {
			spec = &NetworkSpec{}
			config.Networks[name] = spec
		}
		if spec.Egress == "" {
			spec.Egress = "closed"
		}
	}
}

func ShimUpstreamSet(config *Config) bool {
	return config.ShimCfg != nil && config.ShimCfg.UpstreamContainer != ""
}

func HasReservedDebugContainer(config *Config) bool {
	for _, container := range config.Containers {
		if container.Name == ReservedDebugContainerName {
			return true
		}
	}
	return false
}

func ReservedDebugRuntimeEnabled(containerName string, options Options) bool {
	return options.Mode == HostDebugMode && containerName == ReservedDebugContainerName
}
