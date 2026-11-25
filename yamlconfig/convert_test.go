package yamlconfig_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json/jsontext"
	"os"
	"strings"
	"testing"

	. "github.com/tinfoilsh/tinfoil-config"
	"github.com/tinfoilsh/tinfoil-config/yamlconfig"
)

// The pair in testdata states every kind of value the format has. The YAML is
// written the way a person writes it: unquoted numbers, a null network, both
// forms of an environment entry.
func fullConfig(t *testing.T) (yaml, json string) {
	t.Helper()
	source, err := os.ReadFile("../testdata/full.yml")
	if err != nil {
		t.Fatal(err)
	}
	translated, err := os.ReadFile("../testdata/full.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(source), string(translated)
}

// fullCommit is the source commit the pair pins its first container to.
const fullCommit = "4f1c2d3e5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d"

func TestToJSONIsLiteralAndCanonical(t *testing.T) {
	fullYAML, fullJSON := fullConfig(t)
	got, err := yamlconfig.ToJSON([]byte(fullYAML), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != fullJSON {
		t.Fatalf("JSON form:\n%s\nwant:\n%s", got, fullJSON)
	}
	config, err := yamlconfig.Decode(got, Options{})
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

// Some commits are text that YAML reads as a number when it is not quoted.
// The JSON form states the text as written either way.
func TestToJSONTakesACommitAsWritten(t *testing.T) {
	fullYAML, fullJSON := fullConfig(t)
	if !strings.Contains(fullYAML, "commit: "+fullCommit) || !strings.Contains(fullJSON, `"commit":"`+fullCommit+`"`) {
		t.Fatalf("test config lacks commit %s", fullCommit)
	}
	for name, written := range map[string]string{
		"all digits":        "1234567890123456789012345678901234567890",
		"digits with one e": "12345678901234567890123456789012345678e9",
		"leading zero":      "0000000000000000000000000000000000000017",
		"binary prefix":     "0b00000000000000000000000000000000000011",
		"quoted":            `"1234567890123456789012345678901234567890"`,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := yamlconfig.ToJSON([]byte(strings.Replace(fullYAML, "commit: "+fullCommit, "commit: "+written, 1)), Options{})
			if err != nil {
				t.Fatal(err)
			}
			if want := strings.Replace(fullJSON, fullCommit, strings.Trim(written, `"`), 1); string(got) != want {
				t.Fatalf("JSON form:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestNormalizeGivesOneByteFormPerConfig(t *testing.T) {
	fullYAML, fullJSON := fullConfig(t)
	fromYAML, err := yamlconfig.Normalize([]byte(fullYAML), Options{})
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
		got, err := yamlconfig.Normalize(input, Options{})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(got, fromYAML) {
			t.Fatalf("%s: normalized to different bytes", name)
		}
	}
	legacy := []byte(validConfig)
	got, err := yamlconfig.Normalize(legacy, Options{})
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
		if _, err := yamlconfig.Decode([]byte(pinned(version)), Options{}); err != nil {
			t.Fatalf("YAML for %s: %v", version, err)
		}
	}
	for _, version := range []string{"0.15.5-rc.1" + digest, "0.15.5" + digest, "0.16.0" + digest, "v1.0.0" + digest} {
		yaml := []byte(pinned(version))
		if _, err := yamlconfig.Decode(yaml, Options{}); err == nil || !strings.Contains(err.Error(), "take a JSON config") {
			t.Fatalf("YAML for %s: error = %v, want it refused", version, err)
		}
		converted, err := yamlconfig.ToJSON(yaml, Options{})
		if err != nil {
			t.Fatalf("converting %s: %v", version, err)
		}
		if _, err := yamlconfig.Decode(converted, Options{}); err != nil {
			t.Fatalf("JSON for %s: %v", version, err)
		}
	}
	// A JSON-looking config for an earlier version is still read as YAML.
	legacy := `{"cvm-version": "0.11.0", "shim": {"upstream-port": 8080}, "containers": [{"name": "app", "image": "example.com/app@sha256:` + strings.Repeat("a", 64) + `"}]}`
	if _, err := yamlconfig.Decode([]byte(legacy), Options{}); err != nil {
		t.Fatalf("flow-style config below the JSON version: %v", err)
	}
}

func TestToJSONRefusesWhatItWouldHaveToInterpret(t *testing.T) {
	fullYAML, _ := fullConfig(t)
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
		"commit in another encoding": {replace("commit: "+fullCommit, "commit: !!binary "+base64.StdEncoding.EncodeToString([]byte(fullCommit))), "quote it"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := yamlconfig.ToJSON([]byte(test.config), Options{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}
