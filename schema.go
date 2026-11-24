package tinfoilconfig

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"reflect"
	"strings"
)

const (
	schemaDialect = "https://json-schema.org/draft/2020-12/schema"
	schemaID      = "https://tinfoil.sh/schemas/tinfoil-config.schema.json"
)

// JSONSchema returns the JSON Schema of a JSON config, derived from the Go
// types. It describes structure and types; the rules that relate fields to
// one another are checked by Decode. A YAML config uses the same names and
// nesting, so the schema also serves editors of the YAML form, which
// additionally accepts unquoted numbers where the JSON form takes strings.
func JSONSchema() []byte {
	root := schemaOf(reflect.TypeFor[Config]())
	root["$schema"] = schemaDialect
	root["$id"] = schemaID
	root["title"] = "Tinfoil enclave config"
	root["required"] = []string{"cvm-version", "shim"}
	root["properties"].(map[string]any)["shim"] = schemaOf(shimType)
	encoded, err := json.Marshal(root, json.Deterministic(true))
	if err != nil {
		panic(err)
	}
	indented := jsontext.Value(encoded)
	if err := indented.Indent(); err != nil {
		panic(err)
	}
	return append(bytes.TrimSpace(indented), '\n')
}

func schemaOf(t reflect.Type) map[string]any {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == durationType {
		return map[string]any{"type": "string", "description": "A duration such as 5m or 90s."}
	}
	switch t.Kind() {
	case reflect.Struct:
		properties := map[string]any{}
		for index := range t.NumField() {
			field := t.Field(index)
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			if !field.IsExported() || name == "" || name == "-" {
				continue
			}
			switch {
			case t == reflect.TypeFor[Container]() && name == "env":
				properties[name] = map[string]any{"type": "array", "items": map[string]any{"oneOf": []any{
					map[string]any{"type": "string", "description": "The name of a variable taken from the deployment's environment."},
					map[string]any{"type": "object", "minProperties": 1, "maxProperties": 1, "additionalProperties": map[string]any{"type": "string"}, "description": "One variable and its literal value."},
				}}}
			case t == reflect.TypeFor[GitHubProvenance]() && name == "repository_id":
				properties[name] = map[string]any{"type": "string", "pattern": repositoryIDPattern.String()}
			default:
				properties[name] = schemaOf(field.Type)
			}
		}
		return map[string]any{"type": "object", "additionalProperties": false, "properties": properties}
	case reflect.Slice:
		return map[string]any{"type": "array", "items": schemaOf(t.Elem())}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": schemaOf(t.Elem())}
	case reflect.Interface:
		return map[string]any{"oneOf": []any{map[string]any{"type": "integer"}, map[string]any{"type": "string"}}}
	case reflect.Float64:
		return map[string]any{"type": "string", "pattern": decimalPattern.String(), "description": "A decimal quantity, written as a string."}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int64:
		return map[string]any{"type": "integer"}
	}
	return map[string]any{"type": "string"}
}
