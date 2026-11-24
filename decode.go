package tinfoilconfig

import (
	"bytes"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

const MaxConfigBytes = 1 << 20

// Decode reads and validates a config. A config that pins JSONConfigMinVersion
// or a later cvm-version must be JSON in its canonical form; earlier ones are
// YAML.
func Decode(data []byte, options Options) (*Config, error) {
	if len(data) > MaxConfigBytes {
		return nil, fmt.Errorf("parsing config: input exceeds %d-byte limit", MaxConfigBytes)
	}
	if version, ok := jsonCVMVersion(data); ok && RequiresJSON(version) {
		return decodeJSON(data, options)
	}
	config, err := decodeYAML(data, options)
	if err != nil {
		return nil, err
	}
	if RequiresJSON(config.CVMVersion) {
		return nil, fmt.Errorf("parsing config: cvm-version %s and later take a JSON config; convert this one with ToJSON", JSONConfigMinVersion)
	}
	return config, nil
}

func decodeYAML(data []byte, options Options) (*Config, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("parsing config: multiple YAML documents")
		}
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	shimConfig, err := DecodeShim(&config.ShimRaw)
	if err != nil {
		return nil, fmt.Errorf("parsing shim config: %w", err)
	}
	return finishDecode(&config, shimConfig, options)
}

func finishDecode(config *Config, shimConfig *ShimConfig, options Options) (*Config, error) {
	shimConfig.ExpectedGPUs = config.GPUs
	if shimConfig.UpstreamContainer == "" && len(config.Containers) > 0 {
		shimConfig.UpstreamContainer = config.Containers[0].Name
	}
	config.ShimCfg = shimConfig
	setNetworkDefaults(config)
	if err := Validate(config, options); err != nil {
		return nil, err
	}
	return config, nil
}

func ValidateBytes(data []byte, options Options) error {
	_, err := Decode(data, options)
	return err
}

func setNetworkDefaults(config *Config) {
	for name, spec := range config.Networks {
		if spec == nil {
			config.Networks[name] = &NetworkSpec{Egress: "closed"}
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
