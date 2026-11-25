package yamlconfig

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	tinfoilconfig "github.com/tinfoilsh/tinfoil-config"
	"gopkg.in/yaml.v3"
)

var (
	durationType = reflect.TypeFor[time.Duration]()
	// decimalPattern is how a JSON config writes a fractional quantity, as
	// tinfoilconfig reads it back.
	decimalPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?$`)
)

// Normalize returns the bytes that represent a config everywhere it is
// stored, measured and launched. A config that pins JSONConfigMinVersion or a
// later cvm-version becomes canonical JSON, whether it arrives as YAML or as
// JSON; an earlier one is returned as it is. Either way the config is
// validated.
func Normalize(data []byte, options tinfoilconfig.Options) ([]byte, error) {
	if len(data) > tinfoilconfig.MaxConfigBytes {
		return nil, fmt.Errorf("parsing config: input exceeds %d-byte limit", tinfoilconfig.MaxConfigBytes)
	}
	if version, ok := jsonCVMVersion(data); ok && tinfoilconfig.RequiresJSON(version) {
		canonical := jsontext.Value(bytes.Clone(data))
		if err := canonical.Canonicalize(); err != nil {
			return nil, fmt.Errorf("parsing config: %w", err)
		}
		if _, err := tinfoilconfig.Decode(canonical, options); err != nil {
			return nil, err
		}
		return canonical, nil
	}
	config, err := decodeYAML(data, options)
	if err != nil {
		return nil, err
	}
	if !tinfoilconfig.RequiresJSON(config.CVMVersion) {
		return data, nil
	}
	return ToJSON(data, options)
}

// ToJSON translates a YAML config into its JSON form. The translation is
// literal: every member the YAML states appears under the same name with the
// value the YAML decoder reads for that field, and nothing else is added, so
// defaults stay the reader's. It goes one way only; nothing reads YAML back
// from the result.
func ToJSON(data []byte, options tinfoilconfig.Options) ([]byte, error) {
	fromYAML, err := decodeYAML(data, options)
	if err != nil {
		return nil, err
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if err := validateYAMLTree(&document); err != nil {
		return nil, fmt.Errorf("converting config: %w", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return nil, fmt.Errorf("converting config: missing YAML document")
	}
	tree, err := convertNode(document.Content[0], reflect.TypeFor[tinfoilconfig.Config](), "config")
	if err != nil {
		return nil, fmt.Errorf("converting config: %w", err)
	}
	encoded, err := json.Marshal(tree)
	if err != nil {
		return nil, fmt.Errorf("converting config: %w", err)
	}
	canonical := jsontext.Value(encoded)
	if err := canonical.Canonicalize(); err != nil {
		return nil, fmt.Errorf("converting config: %w", err)
	}
	// The translation must not change what the config means. Reading both
	// forms and comparing them catches any case the walk above gets wrong.
	fromJSON, err := tinfoilconfig.Decode(canonical, options)
	if err != nil {
		return nil, fmt.Errorf("converting config: the JSON form is not valid: %w", err)
	}
	if !reflect.DeepEqual(comparable(fromYAML), comparable(fromJSON)) {
		return nil, fmt.Errorf("converting config: the JSON form does not read the same as the YAML")
	}
	return canonical, nil
}

// comparable returns config without the one thing that differs between a
// YAML and a JSON reading of the same content: the types of literal
// environment values, which a container receives as text either way.
func comparable(config *tinfoilconfig.Config) *tinfoilconfig.Config {
	copied := *config
	copied.Containers = slices.Clone(config.Containers)
	for index := range copied.Containers {
		container := &copied.Containers[index]
		container.Env = slices.Clone(container.Env)
		for position, item := range container.Env {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			text := make(map[string]any, len(entry))
			for name, value := range entry {
				text[name] = fmt.Sprint(value)
			}
			container.Env[position] = text
		}
	}
	return &copied
}

// convertNode returns the JSON value of node read as a field of type t.
func convertNode(node *yaml.Node, t reflect.Type, path string) (any, error) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == durationType {
		return convertDuration(node, path)
	}
	switch t.Kind() {
	case reflect.Struct:
		return convertStruct(node, t, path)
	case reflect.Slice:
		if node.Kind != yaml.SequenceNode {
			return nil, fmt.Errorf("%s must be a sequence", path)
		}
		items := make([]any, 0, len(node.Content))
		for index, item := range node.Content {
			itemPath := fmt.Sprintf("%s[%d]", path, index)
			if isNull(item) {
				return nil, fmt.Errorf("%s must not be null", itemPath)
			}
			value, err := convertNode(item, t.Elem(), itemPath)
			if err != nil {
				return nil, err
			}
			items = append(items, value)
		}
		return items, nil
	case reflect.Map:
		return convertMap(node, t.Elem(), path)
	case reflect.Interface:
		return convertUntyped(node, path)
	case reflect.Float64:
		return convertDecimal(node, path)
	case reflect.String, reflect.Bool, reflect.Int, reflect.Int64:
		if node.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("%s must be a scalar", path)
		}
		value := reflect.New(t)
		if err := node.Decode(value.Interface()); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return value.Elem().Interface(), nil
	}
	return nil, fmt.Errorf("%s has a type with no JSON form", path)
}

func convertStruct(node *yaml.Node, t reflect.Type, path string) (any, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s must be a mapping", path)
	}
	fields := map[string]reflect.StructField{}
	for index := range t.NumField() {
		field := t.Field(index)
		name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
		if field.IsExported() && name != "" && name != "-" {
			fields[name] = field
		}
	}
	members := map[string]any{}
	for index := 0; index < len(node.Content); index += 2 {
		name, item := node.Content[index].Value, node.Content[index+1]
		memberPath := path + "." + name
		field, known := fields[name]
		if !known {
			return nil, fmt.Errorf("%s is not a known field", memberPath)
		}
		if _, repeated := members[name]; repeated {
			return nil, fmt.Errorf("%s is repeated", memberPath)
		}
		if isNull(item) {
			continue
		}
		var value any
		var err error
		switch {
		case t == reflect.TypeFor[tinfoilconfig.Container]() && name == "env":
			value, err = convertEnv(item, memberPath)
		case t == reflect.TypeFor[tinfoilconfig.GitHubProvenance]() && (name == "repository_id" || name == "commit"):
			// IDs and commits are strings in JSON, so that no reader rounds one.
			value = item.Value
		default:
			value, err = convertNode(item, field.Type, memberPath)
		}
		if err != nil {
			return nil, err
		}
		members[name] = value
	}
	return members, nil
}

func convertMap(node *yaml.Node, elem reflect.Type, path string) (any, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s must be a mapping", path)
	}
	members := map[string]any{}
	for index := 0; index < len(node.Content); index += 2 {
		name, item := node.Content[index].Value, node.Content[index+1]
		memberPath := path + "." + name
		if _, repeated := members[name]; repeated {
			return nil, fmt.Errorf("%s is repeated", memberPath)
		}
		if isNull(item) {
			// A network with no settings is the one entry null stands for.
			if elem != reflect.TypeFor[*tinfoilconfig.NetworkSpec]() {
				return nil, fmt.Errorf("%s must not be null", memberPath)
			}
			members[name] = map[string]any{}
			continue
		}
		value, err := convertNode(item, elem, memberPath)
		if err != nil {
			return nil, err
		}
		members[name] = value
	}
	return members, nil
}

// convertEnv translates a container's environment: a name alone stays a
// string, and a literal value becomes the text the container receives.
func convertEnv(node *yaml.Node, path string) (any, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s must be a sequence", path)
	}
	items := make([]any, 0, len(node.Content))
	for index, item := range node.Content {
		itemPath := fmt.Sprintf("%s[%d]", path, index)
		switch item.Kind {
		case yaml.ScalarNode:
			if isNull(item) {
				return nil, fmt.Errorf("%s must not be null", itemPath)
			}
			var name string
			if err := item.Decode(&name); err != nil {
				return nil, fmt.Errorf("%s: %w", itemPath, err)
			}
			items = append(items, name)
		case yaml.MappingNode:
			entry := map[string]any{}
			for position := 0; position < len(item.Content); position += 2 {
				name, value := item.Content[position].Value, item.Content[position+1]
				var decoded any
				if err := value.Decode(&decoded); err != nil || value.Kind != yaml.ScalarNode || isNull(value) {
					return nil, fmt.Errorf("%s.%s must be a scalar value", itemPath, name)
				}
				text := fmt.Sprint(decoded)
				// YAML reads an unquoted 1.10 as the number 1.1 and 0x10 as 16.
				// The JSON form states the text outright, so a value whose
				// text would change is refused here instead of altered.
				if _, isText := decoded.(string); !isText && text != value.Value {
					return nil, fmt.Errorf("%s.%s: %s is read as %s; quote it to say which you mean", itemPath, name, value.Value, text)
				}
				entry[name] = text
			}
			items = append(items, entry)
		default:
			return nil, fmt.Errorf("%s must be a name or a one-key mapping", itemPath)
		}
	}
	return items, nil
}

// convertUntyped translates a field that takes an integer or a string.
func convertUntyped(node *yaml.Node, path string) (any, error) {
	if node.Kind != yaml.ScalarNode {
		return nil, fmt.Errorf("%s must be a scalar", path)
	}
	var decoded any
	if err := node.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	switch decoded.(type) {
	case int, string:
		return decoded, nil
	}
	return nil, fmt.Errorf("%s must be an integer or a string", path)
}

// convertDecimal translates a fractional quantity into the string that holds
// it in JSON, whose numbers are integers only.
func convertDecimal(node *yaml.Node, path string) (any, error) {
	var value float64
	if node.Kind != yaml.ScalarNode || node.Decode(&value) != nil || !decimalPattern.MatchString(node.Value) {
		return nil, fmt.Errorf("%s must be a plain decimal such as 2 or 0.5", path)
	}
	return node.Value, nil
}

func convertDuration(node *yaml.Node, path string) (any, error) {
	var value time.Duration
	if node.Kind != yaml.ScalarNode || node.Decode(&value) != nil {
		return nil, fmt.Errorf("%s must be a duration", path)
	}
	if _, err := time.ParseDuration(node.Value); err == nil {
		return node.Value, nil
	}
	return value.String(), nil
}

func isNull(node *yaml.Node) bool {
	return node.Kind == yaml.ScalarNode && node.Tag == yamlNullTag
}
