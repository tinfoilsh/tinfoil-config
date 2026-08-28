package tinfoilconfig

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

const (
	ReservedDebugContainerName = "tinfoil-debug-toolbox"
	ReservedDebugPort          = "2222/tcp"
	ReservedDebugHostPort      = 2222
)

// ValidationMode selects which producer is allowed to construct the config.
// WorkloadMode is for user-supplied measured YAML. HostDebugMode is only for
// YAML after tinfoild has injected its reserved debug toolbox.
type ValidationMode uint8

const (
	WorkloadMode ValidationMode = iota
	HostDebugMode
)

type Options struct {
	Mode ValidationMode
}

type Config struct {
	CVMVersion string                  `yaml:"cvm-version"`
	ShimRaw    yaml.Node               `yaml:"shim"`
	ShimCfg    *ShimConfig             `yaml:"-"`
	CVMNetwork CVMNetworkConfig        `yaml:"cvm-network"`
	Networks   map[string]*NetworkSpec `yaml:"networks"`
	CPUs       int                     `yaml:"cpus"`
	Memory     int                     `yaml:"memory"`
	GPUs       int                     `yaml:"gpus"`
	Models     []ModelSpec             `yaml:"models"`
	Containers []Container             `yaml:"containers"`
	KBSURL     string                  `yaml:"kbs-url,omitempty"`
	VaultURL   string                  `yaml:"vault-url,omitempty"` // Deprecated: use KBSURL.
}

func (c *Config) KeyBrokerURL() string {
	if c.KBSURL != "" {
		return c.KBSURL
	}
	return c.VaultURL
}

type CVMNetworkConfig struct {
	InboundPorts []int `yaml:"inbound-ports"`
}

type NetworkSpec struct {
	Egress string   `yaml:"egress"`
	Allow  []string `yaml:"allow"`
}

func (n *NetworkSpec) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		if node.Tag != "!!null" {
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
	type alias NetworkSpec
	var raw alias
	if err := node.Decode(&raw); err != nil {
		return err
	}
	*n = NetworkSpec(raw)
	if n.Egress == "" {
		n.Egress = "closed"
	}
	return nil
}

type ModelSpec struct {
	Name      string `yaml:"name,omitempty"`
	Repo      string `yaml:"repo,omitempty"`
	MPK       string `yaml:"mpk,omitempty"`
	MWP       string `yaml:"mwp,omitempty"`
	EMWP      string `yaml:"emwp,omitempty"`
	KeySecret string `yaml:"key-secret,omitempty"`
}

type Container struct {
	Name        string            `yaml:"name"`
	Image       string            `yaml:"image"`
	Command     []string          `yaml:"command,omitempty"`
	Entrypoint  []string          `yaml:"entrypoint,omitempty"`
	WorkingDir  string            `yaml:"working_dir,omitempty"`
	User        string            `yaml:"user,omitempty"`
	Env         []interface{}     `yaml:"env,omitempty"`
	Secrets     []string          `yaml:"secrets,omitempty"`
	Models      []string          `yaml:"models,omitempty"`
	Volumes     []string          `yaml:"volumes,omitempty"`
	Devices     []string          `yaml:"devices,omitempty"`
	CapAdd      []string          `yaml:"cap_add,omitempty"`
	Runtime     string            `yaml:"runtime,omitempty"`
	Networks    []string          `yaml:"networks,omitempty"`
	IPC         string            `yaml:"ipc,omitempty"`
	PidMode     string            `yaml:"pid,omitempty"`
	GPUs        interface{}       `yaml:"gpus,omitempty"`
	ShmSize     string            `yaml:"shm_size,omitempty"`
	Memory      string            `yaml:"memory,omitempty"`
	CPUs        float64           `yaml:"cpus,omitempty"`
	Tmpfs       map[string]string `yaml:"tmpfs,omitempty"`
	ReadOnly    *bool             `yaml:"read_only,omitempty"`
	PidsLimit   *int64            `yaml:"pids_limit,omitempty"`
	Restart     string            `yaml:"restart,omitempty"`
	StopSignal  string            `yaml:"stop_signal,omitempty"`
	StopTimeout *int              `yaml:"stop_timeout,omitempty"`
	Healthcheck *Healthcheck      `yaml:"healthcheck,omitempty"`
	inputFields containerInputFields
}

type containerInputFields struct {
	privileged  bool
	capDrop     bool
	securityOpt bool
}

var containerFields = map[string]bool{
	"name": true, "image": true, "command": true, "entrypoint": true,
	"working_dir": true, "user": true, "env": true, "secrets": true, "models": true,
	"volumes": true, "devices": true, "cap_add": true, "runtime": true,
	"networks": true, "ipc": true, "pid": true, "gpus": true,
	"shm_size": true, "memory": true, "cpus": true, "tmpfs": true,
	"read_only": true, "pids_limit": true, "restart": true,
	"stop_signal": true, "stop_timeout": true, "healthcheck": true,
	"privileged": true, "cap_drop": true, "security_opt": true,
}

func (c *Container) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("container entry must be a mapping")
	}
	var fields containerInputFields
	seen := map[string]bool{}
	for index := 0; index < len(node.Content); index += 2 {
		field := node.Content[index].Value
		if field == "<<" {
			return fmt.Errorf("container YAML merge keys are unsupported")
		}
		if seen[field] {
			return fmt.Errorf("duplicate container field %q", field)
		}
		seen[field] = true
		if !containerFields[field] {
			return fmt.Errorf("unknown container field %q", field)
		}
		switch field {
		case "privileged":
			fields.privileged = true
		case "cap_drop":
			fields.capDrop = true
		case "security_opt":
			fields.securityOpt = true
		}
	}
	type rawContainer Container
	var raw rawContainer
	if err := node.Decode(&raw); err != nil {
		return err
	}
	*c = Container(raw)
	c.inputFields = fields
	return nil
}

type Healthcheck struct {
	Test        []string `yaml:"test"`
	Interval    string   `yaml:"interval,omitempty"`
	Timeout     string   `yaml:"timeout,omitempty"`
	Retries     int      `yaml:"retries,omitempty"`
	StartPeriod string   `yaml:"start_period,omitempty"`
}

func (h *Healthcheck) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("healthcheck must be a mapping")
	}
	allowed := map[string]bool{
		"test": true, "interval": true, "timeout": true,
		"retries": true, "start_period": true,
	}
	seen := map[string]bool{}
	for index := 0; index < len(node.Content); index += 2 {
		field := node.Content[index].Value
		if seen[field] {
			return fmt.Errorf("duplicate healthcheck field %q", field)
		}
		seen[field] = true
		if !allowed[field] {
			return fmt.Errorf("unknown healthcheck field %q", field)
		}
	}
	type rawHealthcheck Healthcheck
	var raw rawHealthcheck
	if err := node.Decode(&raw); err != nil {
		return err
	}
	*h = Healthcheck(raw)
	return nil
}
