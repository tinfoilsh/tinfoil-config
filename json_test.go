package tinfoilconfig

import (
	"os"
	"strings"
	"testing"
)

// fullJSON reads a config that states every kind of value the format has.
// testdata/full.yml is its YAML source; the yamlconfig tests translate one
// into the other.
func fullJSON(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("testdata/full.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestDecodeReadsAJSONConfig(t *testing.T) {
	config, err := Decode([]byte(fullJSON(t)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	app := config.Containers[0]
	if app.CPUs != 1.5 || app.GPUs != 2 || app.Provenance.GitHub.RepositoryID != 123456789 || config.ShimCfg.RateLimit != 0.5 || config.ShimCfg.DrainTimeout.String() != "1m30s" {
		t.Fatalf("decoded values: cpus=%v gpus=%v id=%v rate=%v drain=%v", app.CPUs, app.GPUs, app.Provenance.GitHub.RepositoryID, config.ShimCfg.RateLimit, config.ShimCfg.DrainTimeout)
	}
	if config.Containers[1].GPUs != "0,1" || app.Env[1].(map[string]any)["PORT"] != "8080" {
		t.Fatalf("untyped values: gpus=%v env=%v", config.Containers[1].GPUs, app.Env)
	}
	if config.Networks["app"].Egress != "closed" || config.ShimCfg.TLSMode != "cert-proxy" || config.ShimCfg.UpstreamContainer != "app" {
		t.Fatalf("defaults were not applied on reading: %+v %q %q", config.Networks["app"], config.ShimCfg.TLSMode, config.ShimCfg.UpstreamContainer)
	}
}

func TestJSONConfigMustBeInItsOneForm(t *testing.T) {
	canonical := fullJSON(t)
	replace := func(old, new string) string {
		if !strings.Contains(canonical, old) {
			t.Fatalf("test config lacks %q", old)
		}
		return strings.Replace(canonical, old, new, 1)
	}
	for name, test := range map[string]struct{ config, want string }{
		"whitespace":           {replace(`"cpus":8`, `"cpus": 8`), "canonical form"},
		"trailing newline":     {canonical + "\n", "canonical form"},
		"members out of order": {replace(`"cpus":8,"cvm-network":{"inbound-ports":[8443]},`, `"cvm-network":{"inbound-ports":[8443]},"cpus":8,`), "canonical form"},
		"null member":          {replace(`"memory":16384`, `"keyserver-url":null,"memory":16384`), "null is not allowed"},
		"fraction":             {replace(`"memory":16384`, `"memory":16384.5`), "not an integer"},
		"exponent":             {replace(`"memory":16384`, `"memory":1e4`), "not an integer"},
		"integer too large":    {replace(`"memory":16384`, `"memory":9007199254740992`), "not an integer"},
		"repeated member":      {replace(`"memory":16384`, `"memory":16384,"memory":16384`), "duplicate"},
		"unknown member":       {replace(`"memory":16384`, `"memory":16384,"memoryy":1`), "unknown"},
		"unknown shim member":  {replace(`"upstream-port":8080`, `"upstream-port":8080,"upstream-porty":1`), "unknown"},
		"number for a decimal": {replace(`"cpus":"1.5"`, `"cpus":1`), "must be a string"},
		"number for an ID":     {replace(`"repository_id":"123456789"`, `"repository_id":123456789`), "repository_id"},
		"ID with leading zero": {replace(`"repository_id":"123456789"`, `"repository_id":"0123456789"`), "plain decimal"},
		"number in env":        {replace(`{"PORT":"8080"}`, `{"PORT":8080}`), "must be a string"},
		"unsupported field":    {replace(`"provenance":{"github"`, `"privileged":true,"provenance":{"github"`), "unknown"},
		"second value":         {canonical + canonical, ""},
		"earlier version":      {replace(`"cvm-version":"0.15.5-rc.1@`, `"cvm-version":"0.15.5-rc.0@`), "pins cvm-version"},
		"YAML":                 {"cvm-version: 0.15.5-rc.1@sha256:" + strings.Repeat("a", 64) + "\nshim:\n  upstream-port: 8080\n", "take a JSON config"},
		"not an object":        {"[" + canonical + "]", ""},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Decode([]byte(test.config), Options{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}
