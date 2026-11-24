package tinfoilconfig

import (
	"bytes"
	"encoding/json/jsontext"
	"strings"
	"testing"
)

const jsonPin = "0.15.5-rc.1@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// fullYAML states every kind of value the format has, written the way a
// person writes YAML: unquoted numbers, a null network, both forms of an
// environment entry.
const fullYAML = `
cvm-version: ` + jsonPin + `
cpus: 8
memory: 16384
gpus: 2
shim:
  upstream-port: 8080
  paths: [/v1/chat/completions, /health]
  rate-limit: 0.5
  drain_connections: true
  drain_timeout: 90s
  authenticated-endpoints: []
cvm-network:
  inbound-ports: [8443]
networks:
  app:
  web:
    egress: allowlist
    allow: [example.com]
security:
  include-config-in-attestation: true
  allow-unencrypted-nvlink: true
volumes:
  - name: data
    size: 10GiB
containers:
  - name: app
    image: example.com/app@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    provenance:
      github:
        repository: example/app
        repository_id: 123456789
        workflow: release.yml
        ref: refs/tags/v*
    command: [serve, --port, "8080"]
    env:
      - HF_TOKEN
      - PORT: 8080
      - DEBUG: true
      - VERSION: "1.10"
    networks: [app, web]
    persistent_volumes: [data:/data]
    runtime: nvidia
    gpus: 2
    cpus: 1.5
    read_only: true
    pids_limit: 4096
    stop_timeout: 30
    tmpfs:
      /tmp: size=64m
    healthcheck:
      test: [CMD, "true"]
      retries: 3
  - name: worker
    image: example.com/worker@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
    networks: [app]
    runtime: nvidia
    gpus: "0,1"
`

const fullJSON = `{"containers":[{"command":["serve","--port","8080"],"cpus":"1.5","env":["HF_TOKEN",{"PORT":"8080"},{"DEBUG":"true"},{"VERSION":"1.10"}],"gpus":2,"healthcheck":{"retries":3,"test":["CMD","true"]},"image":"example.com/app@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","name":"app","networks":["app","web"],"persistent_volumes":["data:/data"],"pids_limit":4096,"provenance":{"github":{"ref":"refs/tags/v*","repository":"example/app","repository_id":"123456789","workflow":"release.yml"}},"read_only":true,"runtime":"nvidia","stop_timeout":30,"tmpfs":{"/tmp":"size=64m"}},{"gpus":"0,1","image":"example.com/worker@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","name":"worker","networks":["app"],"runtime":"nvidia"}],"cpus":8,"cvm-network":{"inbound-ports":[8443]},"cvm-version":"` + jsonPin + `","gpus":2,"memory":16384,"networks":{"app":{},"web":{"allow":["example.com"],"egress":"allowlist"}},"security":{"allow-unencrypted-nvlink":true,"include-config-in-attestation":true},"shim":{"authenticated-endpoints":[],"drain_connections":true,"drain_timeout":"90s","paths":["/v1/chat/completions","/health"],"rate-limit":"0.5","upstream-port":8080},"volumes":[{"name":"data","size":"10GiB"}]}`

func TestToJSONIsLiteralAndCanonical(t *testing.T) {
	got, err := ToJSON([]byte(fullYAML), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != fullJSON {
		t.Fatalf("JSON form:\n%s\nwant:\n%s", got, fullJSON)
	}
	config, err := Decode(got, Options{})
	if err != nil {
		t.Fatal(err)
	}
	app := config.Containers[0]
	if app.CPUs != 1.5 || app.GPUs != 2 || app.Provenance.GitHub.RepositoryID != 123456789 || config.ShimCfg.RateLimit != 0.5 || config.ShimCfg.DrainTimeout.String() != "1m30s" {
		t.Fatalf("decoded values: cpus=%v gpus=%v id=%v rate=%v drain=%v", app.CPUs, app.GPUs, app.Provenance.GitHub.RepositoryID, config.ShimCfg.RateLimit, config.ShimCfg.DrainTimeout)
	}
	if config.Networks["app"].Egress != "closed" || config.ShimCfg.TLSMode != "cert-proxy" {
		t.Fatalf("defaults were not applied on reading: %+v %q", config.Networks["app"], config.ShimCfg.TLSMode)
	}
}

func TestNormalizeGivesOneByteFormPerConfig(t *testing.T) {
	fromYAML, err := Normalize([]byte(fullYAML), Options{})
	if err != nil {
		t.Fatal(err)
	}
	indented := jsontext.Value(bytes.Clone(fromYAML))
	if err := indented.Indent(); err != nil {
		t.Fatal(err)
	}
	reordered := strings.Replace(fullJSON, `"cpus":8,`, "", 1)
	reordered = reordered[:len(reordered)-1] + `,"cpus":8}`
	for name, input := range map[string][]byte{"canonical": fromYAML, "indented": indented, "reordered": []byte(reordered)} {
		got, err := Normalize(input, Options{})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(got, fromYAML) {
			t.Fatalf("%s: normalized to different bytes", name)
		}
	}
	legacy := []byte(validConfig)
	got, err := Normalize(legacy, Options{})
	if err != nil || !bytes.Equal(got, legacy) {
		t.Fatalf("a config below the JSON version changed: %v", err)
	}
}

func TestConfigFormatFollowsThePinnedVersion(t *testing.T) {
	pinned := func(version string) string {
		return strings.Replace(validConfig, "cvm-version: 0.11.0", "cvm-version: "+version, 1)
	}
	digest := "@sha256:" + strings.Repeat("a", 64)
	for _, version := range []string{"0.15.4" + digest, "0.15.5-rc.0" + digest, "0.14.19"} {
		if _, err := Decode([]byte(pinned(version)), Options{}); err != nil {
			t.Fatalf("YAML for %s: %v", version, err)
		}
	}
	for _, version := range []string{"0.15.5-rc.1" + digest, "0.15.5" + digest, "0.16.0" + digest, "v1.0.0" + digest} {
		yaml := []byte(pinned(version))
		if _, err := Decode(yaml, Options{}); err == nil || !strings.Contains(err.Error(), "take a JSON config") {
			t.Fatalf("YAML for %s: error = %v, want it refused", version, err)
		}
		converted, err := ToJSON(yaml, Options{})
		if err != nil {
			t.Fatalf("converting %s: %v", version, err)
		}
		if _, err := Decode(converted, Options{}); err != nil {
			t.Fatalf("JSON for %s: %v", version, err)
		}
	}
	// A JSON-looking config for an earlier version is still read as YAML.
	legacy := `{"cvm-version": "0.11.0", "shim": {"upstream-port": 8080}, "containers": [{"name": "app", "image": "example.com/app@sha256:` + strings.Repeat("a", 64) + `"}]}`
	if _, err := Decode([]byte(legacy), Options{}); err != nil {
		t.Fatalf("flow-style config below the JSON version: %v", err)
	}
}

func TestJSONConfigMustBeInItsOneForm(t *testing.T) {
	canonical, err := ToJSON([]byte(fullYAML), Options{})
	if err != nil {
		t.Fatal(err)
	}
	replace := func(old, new string) string {
		if !strings.Contains(string(canonical), old) {
			t.Fatalf("test config lacks %q", old)
		}
		return strings.Replace(string(canonical), old, new, 1)
	}
	for name, test := range map[string]struct{ config, want string }{
		"whitespace":           {replace(`"cpus":8`, `"cpus": 8`), "canonical form"},
		"trailing newline":     {string(canonical) + "\n", "canonical form"},
		"members out of order": {replace(`"cpus":8,"cvm-network":{"inbound-ports":[8443]},`, `"cvm-network":{"inbound-ports":[8443]},"cpus":8,`), "canonical form"},
		"null member":          {replace(`"memory":16384`, `"keyserver-url":null,"memory":16384`), "null is not allowed"},
		"fraction":             {replace(`"memory":16384`, `"memory":16384.5`), "not an integer"},
		"exponent":             {replace(`"memory":16384`, `"memory":1e4`), "not an integer"},
		"integer too large":    {replace(`"memory":16384`, `"memory":9007199254740992`), "not an integer"},
		"repeated member":      {replace(`"memory":16384`, `"memory":16384,"memory":16384`), "already defined"},
		"unknown member":       {replace(`"memory":16384`, `"memory":16384,"memoryy":1`), "unknown"},
		"unknown shim member":  {replace(`"upstream-port":8080`, `"upstream-port":8080,"upstream-porty":1`), "unknown"},
		"number for a decimal": {replace(`"cpus":"1.5"`, `"cpus":1`), "must be a string"},
		"number for an ID":     {replace(`"repository_id":"123456789"`, `"repository_id":123456789`), "repository_id"},
		"ID with leading zero": {replace(`"repository_id":"123456789"`, `"repository_id":"0123456789"`), "plain decimal"},
		"number in env":        {replace(`{"PORT":"8080"}`, `{"PORT":8080}`), "must be a string"},
		"unsupported field":    {replace(`"provenance":{"github"`, `"privileged":true,"provenance":{"github"`), "unknown"},
		"second value":         {string(canonical) + string(canonical), ""},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Decode([]byte(test.config), Options{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestToJSONRefusesWhatItWouldHaveToInterpret(t *testing.T) {
	replace := func(old, new string) string {
		if !strings.Contains(fullYAML, old) {
			t.Fatalf("test config lacks %q", old)
		}
		return strings.Replace(fullYAML, old, new, 1)
	}
	for name, test := range map[string]struct{ config, want string }{
		"number that loses its text": {replace(`VERSION: "1.10"`, `VERSION: 1.10`), "quote it"},
		"octal read as decimal":      {replace(`PORT: 8080`, `PORT: 0o17`), "quote it"},
		"null value":                 {replace(`PORT: 8080`, `PORT:`), "must be a scalar"},
		"alias":                      {replace("  web:\n    egress: allowlist\n    allow: [example.com]", "  web: &web\n    egress: allowlist\n    allow: [example.com]\n  other: *web"), "aliases are unsupported"},
		"exponent for a decimal":     {replace(`cpus: 1.5`, `cpus: 15e-1`), "plain decimal"},
		"null list item":             {replace(`command: [serve, --port, "8080"]`, `command: [serve, null]`), "must not be null"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ToJSON([]byte(test.config), Options{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}
