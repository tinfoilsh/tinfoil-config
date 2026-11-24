package yamlconfig

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	tinfoilconfig "github.com/tinfoilsh/tinfoil-config"
)

// The YAML reader lists the top-level members itself. A member added to the
// config must be added there too, or YAML that states it would be refused.
func TestYAMLReaderKnowsEveryTopLevelMember(t *testing.T) {
	members := func(t reflect.Type) []string {
		var names []string
		for index := range t.NumField() {
			if name, _, _ := strings.Cut(t.Field(index).Tag.Get("yaml"), ","); name != "" && name != "-" {
				names = append(names, name)
			}
		}
		slices.Sort(names)
		return names
	}
	want, got := members(reflect.TypeFor[tinfoilconfig.Config]()), members(reflect.TypeFor[yamlConfig]())
	if !slices.Equal(got, want) {
		t.Fatalf("YAML reader members = %v, config members = %v", got, want)
	}
	containerMembers := members(reflect.TypeFor[tinfoilconfig.Container]())
	known := slices.Clone(containerFields)
	slices.Sort(known)
	if !slices.Equal(known, containerMembers) {
		t.Fatalf("YAML reader container fields = %v, container members = %v", known, containerMembers)
	}
}
