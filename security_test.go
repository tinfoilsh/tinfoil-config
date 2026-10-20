package tinfoilconfig

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDecodeSecurityPolicy(t *testing.T) {
	for _, test := range []struct {
		name    string
		fields  string
		want    bool
		wantErr string
	}{
		{name: "omitted"},
		{name: "empty", fields: "security: {}\n"},
		{name: "null section", fields: "security: null\n"},
		{name: "null flag", fields: "security:\n  allow-unencrypted-nvlink: null\n"},
		{name: "disabled", fields: "security:\n  allow-unencrypted-nvlink: false\n"},
		{name: "enabled", fields: "security:\n  allow-unencrypted-nvlink: true\n", want: true},
		{name: "invalid value", fields: "security:\n  allow-unencrypted-nvlink: invalid\n", wantErr: "cannot unmarshal"},
		{name: "invalid section", fields: "security: true\n", wantErr: "cannot unmarshal"},
		{name: "unknown field", fields: "security:\n  allow-unencrypted-nvlinks: true\n", wantErr: "field allow-unencrypted-nvlinks not found"},
		{name: "top-level flag", fields: "allow-unencrypted-nvlink: true\n", wantErr: "field allow-unencrypted-nvlink not found"},
		{name: "duplicate flag", fields: "security:\n  allow-unencrypted-nvlink: true\n  allow-unencrypted-nvlink: false\n", wantErr: "already defined"},
		{name: "duplicate section", fields: "security: {}\nsecurity:\n  allow-unencrypted-nvlink: true\n", wantErr: "already defined"},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, err := Decode([]byte(validConfig+test.fields), Options{})
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("Decode error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if config.Security.AllowUnencryptedNVLink != test.want {
				t.Fatalf("allow-unencrypted-nvlink = %v, want %v", config.Security.AllowUnencryptedNVLink, test.want)
			}
			encoded, err := yaml.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			roundTrip, err := Decode(encoded, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if roundTrip.Security != config.Security {
				t.Fatal("round trip changed the security policy")
			}
			if !test.want && strings.Contains(string(encoded), "security:") {
				t.Fatal("default security policy changed serialized config")
			}
		})
	}
}
