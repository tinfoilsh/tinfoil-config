package tinfoilconfig

import (
	"fmt"
	"strings"
	"testing"
)

const validConfig = `
cvm-version: 0.11.0
cpus: 8
memory: 16384
shim:
  upstream-port: 8080
networks:
  app:
    egress: closed
containers:
  - name: app
    image: example.com/app@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    networks: [app]
`

func TestDecodeValidation(t *testing.T) {
	for _, test := range []struct {
		name    string
		yaml    string
		options Options
		want    string
	}{
		{name: "valid", yaml: validConfig},
		{name: "unknown top-level field", yaml: validConfig + "unknown: true\n", want: "field unknown not found"},
		{name: "legacy Vault URL", yaml: validConfig + "vault-url: https://vault.example\n", want: "field vault-url not found"},
		{name: "unknown container field", yaml: strings.Replace(validConfig, "networks: [app]", "networks: [app]\n    typo: true", 1), want: "unknown container field"},
		{name: "mutable image", yaml: strings.Replace(validConfig, "example.com/app@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "example.com/app:latest", 1), want: "immutable digest"},
		{name: "host socket rejected", yaml: strings.Replace(validConfig, "networks: [app]", "networks: [app]\n    volumes: [/run/docker.sock:/var/run/docker.sock]", 1), want: "named volume"},
		{name: "trusted debug socket accepted", options: Options{Mode: HostDebugMode}, yaml: strings.Replace(validConfig, "name: app\n    image", fmt.Sprintf("name: %s\n    volumes: [/run/docker.sock:/var/run/docker.sock]\n    image", ReservedDebugContainerName), 1)},
		{name: "other host socket rejected in debug mode", options: Options{Mode: HostDebugMode}, yaml: strings.Replace(validConfig, "networks: [app]", "networks: [app]\n    volumes: [/etc:/host]", 1), want: "named volume"},
		{name: "multiple documents", yaml: validConfig + "\n---\n{}\n", want: "multiple YAML documents"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := Decode([]byte(test.yaml), test.options)
			if test.want == "" && err != nil {
				t.Fatal(err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestDecodeKBSURL(t *testing.T) {
	config, err := Decode([]byte(validConfig+"kbs-url: https://kbs.example\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if config.KBSURL != "https://kbs.example" {
		t.Fatalf("KBS URL = %q", config.KBSURL)
	}
}

func TestDecodeSetsDefaults(t *testing.T) {
	config, err := Decode([]byte(validConfig), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if config.ShimCfg.UpstreamContainer != "app" {
		t.Fatalf("upstream container = %q", config.ShimCfg.UpstreamContainer)
	}
	if config.ShimCfg.TLSMode != "cert-proxy" || !config.ShimCfg.PublishAttestation {
		t.Fatalf("shim defaults = %#v", config.ShimCfg)
	}
}

func TestDecodeValidatesModelAccess(t *testing.T) {
	const ref = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa_4096_0eefa619-50b7-588f-a072-d405fb439d36"
	base := strings.Replace(validConfig,
		"containers:\n",
		"models:\n  - name: private-model\n    repo: org/model@revision\n    emwp: "+ref+"\n    key-secret: MODEL_KEY\ncontainers:\n",
		1,
	)

	for _, test := range []struct {
		name string
		yaml string
		want string
	}{
		{name: "explicit grant", yaml: strings.Replace(base, "networks: [app]", "networks: [app]\n    models: [private-model]", 1)},
		{name: "encrypted model without grant", yaml: base, want: "requires an explicit container grant"},
		{name: "unknown model", yaml: strings.Replace(base, "networks: [app]", "networks: [app]\n    models: [unknown]", 1), want: "is not declared"},
		{name: "duplicate grant", yaml: strings.Replace(base, "networks: [app]", "networks: [app]\n    models: [private-model, private-model]", 1), want: "is duplicated"},
		{name: "invalid name", yaml: strings.Replace(strings.Replace(base, "private-model", "../private", 1), "networks: [app]", "networks: [app]\n    models: [../private]", 1), want: "is invalid"},
		{name: "duplicate model", yaml: strings.Replace(strings.Replace(base, "containers:\n", "  - name: private-model\n    mwp: "+ref+"\ncontainers:\n", 1), "networks: [app]", "networks: [app]\n    models: [private-model]", 1), want: "duplicates"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := Decode([]byte(test.yaml), Options{})
			if test.want == "" && err != nil {
				t.Fatal(err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}

	plaintext := strings.Replace(validConfig, "containers:\n", "models:\n  - name: public-model\n    mwp: "+ref+"\ncontainers:\n", 1)
	config, err := Decode([]byte(plaintext), Options{})
	if err != nil {
		t.Fatalf("legacy plaintext model without grant: %v", err)
	}
	if ModelIsIsolated(config, "public-model") {
		t.Fatal("legacy plaintext model is isolated")
	}

	isolatedPlaintext := strings.Replace(plaintext, "networks: [app]", "networks: [app]\n    models: [public-model]", 1)
	config, err = Decode([]byte(isolatedPlaintext), Options{})
	if err != nil {
		t.Fatalf("isolated plaintext model: %v", err)
	}
	if !ModelIsIsolated(config, "public-model") {
		t.Fatal("explicitly granted plaintext model is not isolated")
	}

	namelessPlaintext := strings.Replace(plaintext, "  - name: public-model\n    mwp:", "  - mwp:", 1)
	if _, err := Decode([]byte(namelessPlaintext), Options{}); err != nil {
		t.Fatalf("legacy nameless plaintext model: %v", err)
	}
}

func TestDecodeShimRejectsNilNode(t *testing.T) {
	_, err := DecodeShim(nil)
	if err == nil || !strings.Contains(err.Error(), "missing YAML document") {
		t.Fatalf("error = %v, want missing YAML document", err)
	}
}
