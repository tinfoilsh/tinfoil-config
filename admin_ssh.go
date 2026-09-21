package tinfoilconfig

import (
	"fmt"
	"slices"
)

const AdminSSHGuestPort = 22

// AdminSSHMapping identifies the sole production admin SSH mapping. The host
// allocates its external SSHPort separately; it forwards to GuestPort in the CVM.
type AdminSSHMapping struct {
	Container     string
	GuestPort     int
	ContainerPort int
}

// AdminSSH resolves the measured opt-in shared by guest and host: inbound port
// 22, cvm_admin, and a declared 22:22 port mapping. It does not enable debug mode
// or allocate a host port. Nil means there is no opted-in admin SSH mapping.
// Runtime callers must use a fully validated config and suppress this production
// mapping when running the separate debug-toolbox profile.
func AdminSSH(config *Config) (*AdminSSHMapping, error) {
	if config == nil {
		return nil, fmt.Errorf("missing config")
	}
	if !slices.Contains(config.CVMNetwork.InboundPorts, AdminSSHGuestPort) {
		return nil, nil
	}
	var selected *AdminSSHMapping
	for _, container := range config.Containers {
		if !container.CVMAdmin {
			continue
		}
		ports, err := ParsePorts(container.Ports)
		if err != nil {
			return nil, err
		}
		for _, mapping := range ports {
			if mapping.Host != AdminSSHGuestPort {
				continue
			}
			if mapping.Container != AdminSSHGuestPort || len(container.Networks) == 0 {
				return nil, fmt.Errorf("direct admin SSH requires 22:22 on an attached container network")
			}
			if selected != nil {
				return nil, fmt.Errorf("direct admin SSH requires one unique container mapping")
			}
			selected = &AdminSSHMapping{Container: container.Name, GuestPort: mapping.Host, ContainerPort: mapping.Container}
		}
	}
	if selected != nil {
		if err := requireAttestedKeysRuntime(config); err != nil {
			return nil, err
		}
	}
	return selected, nil
}
