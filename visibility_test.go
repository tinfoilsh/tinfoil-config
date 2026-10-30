package tinfoilconfig

import (
	"strings"
	"testing"
)

func TestConfigVisibility(t *testing.T) {
	for _, version := range []string{"0.11.0", "0.15.2", "0.15.3-beta.1", "0.15.3-rc.0", "0.15.3-rc.1", "v0.15.3-rc.2", "0.15.3-rc.10", "0.15.3", "0.15.4-rc.1", "0.16.0-alpha.1", "0.15.3-rc.1+cpu"} {
		modern := !strings.HasPrefix(version, "0.11.") && version != "0.15.2" && version != "0.15.3-beta.1" && version != "0.15.3-rc.0"
		for _, visibility := range []string{"", VisibilityPublic, VisibilityPrivate, "true", "invalid"} {
			t.Run(version+"/"+visibility, func(t *testing.T) {
				pin := version + "@sha256:" + strings.Repeat("a", 64)
				data := strings.Replace(validConfig, "cvm-version: 0.11.0", "cvm-version: "+pin, 1)
				if visibility != "" {
					data += "\nvisibility: " + visibility + "\n"
				}
				config, err := Decode([]byte(data), Options{})
				invalid := visibility == "true" || visibility == "invalid" || (visibility != "" && !modern)
				if invalid {
					if err == nil {
						t.Fatal("unsupported visibility accepted")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				public := visibility == VisibilityPublic || (visibility == "" && !modern)
				if config.IsPublic() != public || config.DigestOnlyAttestation() != modern {
					t.Fatalf("public=%v, digest-only=%v; want %v, %v", config.IsPublic(), config.DigestOnlyAttestation(), public, modern)
				}
			})
		}
	}
}
