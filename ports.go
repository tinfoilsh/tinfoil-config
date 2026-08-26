package tinfoilconfig

import (
	"fmt"
	"strconv"
	"strings"
)

type PortMapping struct {
	Host      int
	Container int
}

// ParsePorts parses compose-style "<host>:<container>" entries, so no host-IP
// prefix, no ranges, no /udp suffix.
func ParsePorts(ports []string) ([]PortMapping, error) {
	parsed := make([]PortMapping, 0, len(ports))
	for index, spec := range ports {
		host, container, _ := strings.Cut(spec, ":")
		mapping := PortMapping{Host: parsePort(host), Container: parsePort(container)}
		if mapping.Host == 0 || mapping.Container == 0 {
			return nil, fmt.Errorf(`ports[%d] %q must be "<host>:<container>" with ports in 1..65535`, index, spec)
		}
		parsed = append(parsed, mapping)
	}
	return parsed, nil
}

func parsePort(field string) int {
	port, err := strconv.Atoi(field)
	if err != nil || port < 1 || port > 65535 {
		return 0
	}
	return port
}
