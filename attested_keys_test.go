package tinfoilconfig

import (
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func keyConfig() string {
	return strings.Replace(validConfig, "0.11.0", MinCVMVersionAttestedKeys, 1) +
		"    keys: [host-ssh]\nattested-keys:\n  - id: host-ssh\n    key: ecdsa-p256\n"
}

func TestAttestedKeyConfig(t *testing.T) {
	valid := keyConfig()
	for _, tc := range []struct{ name, input, want string }{
		{"valid", valid, ""},
		{"ed25519", strings.Replace(valid, "ecdsa-p256", "ed25519", 1), ""},
		{"x25519", strings.Replace(valid, "ecdsa-p256", "x25519", 1), ""},
		{"ownership", valid + "    uid: 1000\n    gid: 1001\n", ""},
		{"old runtime", strings.Replace(valid, MinCVMVersionAttestedKeys, "0.14.9", 1), "require official cvm-version"},
		{"unreleased runtime", strings.Replace(valid, MinCVMVersionAttestedKeys, "0.15.0-rc1", 1), "require official cvm-version"},
		{"next major", strings.Replace(valid, MinCVMVersionAttestedKeys, "1.0.0", 1), ""},
		{"missing id", strings.ReplaceAll(valid, "host-ssh", ""), "safe lowercase"},
		{"reserved tls", strings.ReplaceAll(valid, "host-ssh", "tls"), "safe lowercase"},
		{"reserved hpke", strings.ReplaceAll(valid, "host-ssh", "hpke"), "safe lowercase"},
		{"path traversal", strings.ReplaceAll(valid, "host-ssh", "../other"), "safe lowercase"},
		{"unknown algorithm", strings.Replace(valid, "ecdsa-p256", "rsa", 1), "unsupported"},
		{"unknown format", valid + "    credential: openssh\n", "field credential not found"},
		{"duplicate declaration", valid + "  - id: host-ssh\n    key: ed25519\n", "declared twice"},
		{"missing grant", strings.Replace(valid, "    keys: [host-ssh]\n", "", 1), "exactly one container grant"},
		{"unknown grant", strings.Replace(valid, "keys: [host-ssh]", "keys: [other]", 1), "unknown attested key"},
		{"duplicate grant", strings.Replace(valid, "keys: [host-ssh]", "keys: [host-ssh, host-ssh]", 1), "duplicate or shared grant"},
		{"negative uid", valid + "    uid: -1\n", "uid and gid"},
		{"large gid", valid + "    gid: 65535\n", "uid and gid"},
		{"run tmpfs", strings.Replace(valid, "    keys:", "    tmpfs: {/run: rw}\n    keys:", 1), ""},
		{"key tmpfs", strings.Replace(valid, "    keys:", "    tmpfs: {/run/tinfoil/keys: rw}\n    keys:", 1), "conflicts with attested key"},
		{"parent tmpfs", strings.Replace(valid, "    keys:", "    tmpfs: {/run/tinfoil: rw}\n    keys:", 1), "conflicts with attested key"},
		{"child tmpfs", strings.Replace(valid, "    keys:", "    tmpfs: {/run/tinfoil/keys/host-ssh/private_key.pem: rw}\n    keys:", 1), "conflicts with attested key"},
		{"volume parent", strings.Replace(valid, "    keys:", "    volumes: [state:/run]\n    keys:", 1) + "volumes: [{name: state}]\n", "conflicts with attested key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Decode([]byte(tc.input), Options{})
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("got %v, want %q", err, tc.want)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := yaml.Marshal(cfg)
			if err != nil {
				t.Fatal(err)
			}
			roundTrip, err := Decode(encoded, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if len(roundTrip.AttestedKeys) != 1 || roundTrip.AttestedKeys[0] != cfg.AttestedKeys[0] || len(roundTrip.Containers[0].Keys) != 1 {
				t.Fatal("round trip dropped declaration or grant")
			}
		})
	}
}

func TestAttestedKeyLimitsAndSharing(t *testing.T) {
	cfg, err := Decode([]byte(keyConfig()), Options{})
	if err != nil {
		t.Fatal(err)
	}
	other := cfg.Containers[0]
	other.Name = "other"
	cfg.Containers = append(cfg.Containers, other)
	if err := Validate(cfg, Options{}); err == nil || !strings.Contains(err.Error(), "shared grant") {
		t.Fatalf("shared grant accepted: %v", err)
	}
	cfg.Containers = cfg.Containers[:1]
	cfg.AttestedKeys, cfg.Containers[0].Keys = nil, nil
	for i := 0; i <= MaxAttestedKeys; i++ {
		id := fmt.Sprintf("key-%d", i)
		cfg.AttestedKeys = append(cfg.AttestedKeys, AttestedKey{ID: id, Key: KeyEd25519})
		cfg.Containers[0].Keys = append(cfg.Containers[0].Keys, id)
	}
	if err := Validate(cfg, Options{}); err == nil || !strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("excessive keys accepted: %v", err)
	}
}

func TestAdminSSH(t *testing.T) {
	base := strings.Replace(validConfig, "0.11.0", MinCVMVersionAttestedKeys, 1) + "    cvm_admin: true\n    ports: ['22:22', '3000:3000']\ncvm-network:\n  inbound-ports: [22]\n"
	for _, tc := range []struct {
		name, input string
		enabled     bool
		want        string
	}{
		{"enabled", base, true, ""},
		{"old image", strings.Replace(base, MinCVMVersionAttestedKeys, "0.14.9", 1), false, "require official cvm-version"},
		{"no inbound opt-in", strings.Replace(base, "inbound-ports: [22]", "inbound-ports: []", 1), false, ""},
		{"ordinary container", strings.Replace(base, "cvm_admin: true", "cvm_admin: false", 1), false, ""},
		{"different published port", strings.Replace(base, "'22:22'", "'2022:22'", 1), false, ""},
		{"wrong target", strings.Replace(base, "'22:22'", "'22:2222'", 1), false, "requires 22:22"},
		{"duplicate mapping", strings.Replace(base, "'3000:3000'", "'22:22'", 1), false, "already published"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Decode([]byte(tc.input), Options{})
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("got %v, want %q", err, tc.want)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			mapping, err := AdminSSH(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if (mapping != nil) != tc.enabled {
				t.Fatalf("mapping = %+v", mapping)
			}
			if mapping != nil && (mapping.Container != "app" || mapping.GuestPort != 22 || mapping.ContainerPort != 22) {
				t.Fatalf("wrong target: %+v", mapping)
			}
		})
	}
}
