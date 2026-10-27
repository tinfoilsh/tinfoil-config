package tinfoilconfig

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// validateWorkloadPolicy checks the measured handoff, not application rules.
// Applications with their own authentication need no inference sidecar.
func validateWorkloadPolicy(config *Config) error {
	for _, container := range config.Containers {
		values := make(map[string]string)
		passed := make(map[string]bool)
		for _, item := range container.Env {
			switch item := item.(type) {
			case string:
				passed[item] = true
			case map[string]interface{}:
				for name, value := range item {
					if _, duplicate := values[name]; duplicate && strings.HasPrefix(name, "SIDECAR_") {
						return fmt.Errorf("container %s repeats %s", container.Name, name)
					}
					values[name] = fmt.Sprint(value)
				}
			}
		}
		raw, configured := values["SIDECAR_POLICY"]
		guarded := slices.Contains(container.Entrypoint, "--policy-version=1")
		for name := range passed {
			if strings.HasPrefix(name, "SIDECAR_") {
				return fmt.Errorf("container %s must measure %s as a literal environment value", container.Name, name)
			}
		}
		for _, name := range container.Secrets {
			if strings.HasPrefix(name, "SIDECAR_") {
				return fmt.Errorf("container %s cannot take %s from secrets", container.Name, name)
			}
		}
		if !configured && !guarded {
			continue
		}
		if !configured || !guarded || len(container.Entrypoint) < 2 {
			return fmt.Errorf("container %s requires both a measured SIDECAR_POLICY and an explicit sidecar entrypoint guarded by --policy-version=1", container.Name)
		}
		var policy struct {
			Version       int    `json:"version"`
			Authenticated *bool  `json:"authenticated"`
			ControlPlane  string `json:"control-plane"`
		}
		if err := json.Unmarshal([]byte(raw), &policy); err != nil || policy.Version != 1 || policy.Authenticated == nil {
			return fmt.Errorf("container %s requires SIDECAR_POLICY version 1 and explicit authenticated", container.Name)
		}
		if config.ShimCfg != nil && container.Name == config.ShimCfg.UpstreamContainer && values["SIDECAR_LISTEN"] != strconv.Itoa(config.ShimCfg.UpstreamPort) {
			return fmt.Errorf("container %s SIDECAR_LISTEN must match shim.upstream-port", container.Name)
		}
		if !*policy.Authenticated {
			continue
		}
		if !passed["DOMAIN"] && values["DOMAIN"] == "" {
			return fmt.Errorf("container %s must explicitly pass deployment DOMAIN", container.Name)
		}
		control, err := url.Parse(policy.ControlPlane)
		if err != nil || control.Scheme != "https" || control.Hostname() == "" {
			return fmt.Errorf("container %s requires an HTTPS control-plane", container.Name)
		}
		allowed := false
		for _, name := range container.Networks {
			network := config.Networks[name]
			if network == nil {
				continue
			}
			if network.Egress == "open" || (network.Egress == "allowlist" && slices.Contains(network.Allow, control.Hostname())) {
				allowed = true
			}
		}
		if !allowed {
			return fmt.Errorf("container %s must allow control-plane/JWKS egress to %s", container.Name, control.Hostname())
		}
	}
	return nil
}
