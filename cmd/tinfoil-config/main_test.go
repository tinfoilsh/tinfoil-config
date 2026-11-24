package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise a separately linked CLI process so digest support cannot be
// satisfied incidentally by imports in the Go test binary.
func TestStandaloneCLIValidatesSHA256Image(t *testing.T) {
	tempDir := t.TempDir()
	binary := filepath.Join(tempDir, "tinfoil-config")
	if output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	config := filepath.Join(tempDir, "config.yml")
	data := []byte(`
cvm-version: 0.11.0
shim:
  upstream-port: 8080
containers:
  - name: app
    image: example.com/app@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
`)
	if err := os.WriteFile(config, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(binary, config).CombinedOutput(); err != nil {
		t.Fatalf("run CLI: %v\n%s", err, output)
	}
	// A config for a JSON version is refused as YAML and accepted once the
	// CLI has translated it.
	pinned := bytes.Replace(data, []byte("cvm-version: 0.11.0"), []byte("cvm-version: 0.15.5-rc.1@sha256:"+strings.Repeat("a", 64)), 1)
	if err := os.WriteFile(config, pinned, 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(binary, config).CombinedOutput(); err == nil {
		t.Fatalf("YAML for a JSON version was accepted:\n%s", output)
	}
	translated, err := exec.Command(binary, "--json", config).Output()
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if err := os.WriteFile(config, translated, 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(binary, config).CombinedOutput(); err != nil {
		t.Fatalf("run CLI on JSON: %v\n%s", err, output)
	}
}
