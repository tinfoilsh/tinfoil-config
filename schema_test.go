package tinfoilconfig

import (
	"bytes"
	"os"
	"testing"
)

func TestSchemaFileIsCurrent(t *testing.T) {
	committed, err := os.ReadFile("tinfoil-config.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(committed, JSONSchema()) {
		t.Fatal("tinfoil-config.schema.json is out of date; regenerate it with: go run ./cmd/tinfoil-config --schema > tinfoil-config.schema.json")
	}
}
