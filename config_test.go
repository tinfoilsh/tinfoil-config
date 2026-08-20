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
