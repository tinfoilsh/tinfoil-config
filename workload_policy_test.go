package tinfoilconfig

import (
	"strings"
	"testing"
)

const workloadConfig = `
cvm-version: 0.11.0
shim:
  application-policy: workload
  upstream-port: 8001
networks:
  credentials:
    egress: allowlist
    allow: [api.tinfoil.sh]
containers:
  - name: app
    image: example.com/app@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    entrypoint: [/opt/tinfoil/inference-sidecar, --policy-version=1, vllm, serve]
    networks: [credentials]
    env:
      - DOMAIN
      - SIDECAR_LISTEN: '8001'
      - SIDECAR_POLICY: '{"version":1,"authenticated":true,"control-plane":"https://api.tinfoil.sh"}'
`

func TestWorkloadPolicyImageCompatibility(t *testing.T) {
	for _, tc := range []struct{ name, config, want string }{
		{"migrated", workloadConfig, ""},
		{"legacy auth", strings.Replace(workloadConfig, "  application-policy: workload", "  application-policy: workload\n  authenticated: true", 1), "shim.authenticated"},
		{"legacy disabled auth", strings.Replace(workloadConfig, "  application-policy: workload", "  application-policy: workload\n  authenticated: false", 1), "shim.authenticated"},
		{"legacy paths", strings.Replace(workloadConfig, "  application-policy: workload", "  application-policy: workload\n  paths: []", 1), "shim.paths"},
		{"missing owner", strings.Replace(workloadConfig, "  application-policy: workload\n", "", 1), "application-policy"},
		{"old sidecar entrypoint", strings.Replace(workloadConfig, ", --policy-version=1", "", 1), "guarded"},
		{"missing domain", strings.Replace(workloadConfig, "      - DOMAIN\n", "", 1), "DOMAIN"},
		{"closed egress", strings.Replace(workloadConfig, "allow: [api.tinfoil.sh]", "allow: [other.example]", 1), "egress"},
		{"external policy", strings.Replace(workloadConfig, "      - DOMAIN", "      - DOMAIN\n      - SIDECAR_POLICY", 1), "literal"},
		{"secret override", workloadConfig + "    secrets: [SIDECAR_POLICY]\n", "secrets"},
		{"different listener", strings.Replace(workloadConfig, "SIDECAR_LISTEN: '8001'", "SIDECAR_LISTEN: '9000'", 1), "upstream-port"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decode([]byte(tc.config), Options{WorkloadPolicy: true})
			if tc.want == "" && err != nil {
				t.Fatal(err)
			}
			if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("error=%v, want %q", err, tc.want)
			}
		})
	}
	legacy := strings.Replace(workloadConfig, "  application-policy: workload", "  authenticated: true", 1)
	if _, err := Decode([]byte(legacy), Options{}); err != nil {
		t.Fatalf("legacy deployed image configuration must remain supported: %v", err)
	}
}

func TestTunnelOptInRequiresPublishedPort(t *testing.T) {
	for _, ports := range []string{"", "    ports: ['2300:22']\n"} {
		_, err := Decode([]byte(workloadConfig+ports+"    tunnel-ports: [2301]\n"), Options{WorkloadPolicy: true})
		if err == nil || !strings.Contains(err.Error(), "tunnel-ports") {
			t.Fatalf("unpublished tunnel accepted: %v", err)
		}
	}
	if _, err := Decode([]byte(workloadConfig+"    ports: ['2300:22']\n    tunnel-ports: [2300]\n"), Options{WorkloadPolicy: true}); err != nil {
		t.Fatal(err)
	}
}
