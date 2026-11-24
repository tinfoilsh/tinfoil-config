package tinfoilconfig

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/creasty/defaults"
	"golang.org/x/mod/semver"
)

// JSONConfigMinVersion is the first cvm-version whose config is JSON. A
// config that pins it or a later version is measured, stored and read as
// canonical JSON, and YAML is refused; earlier versions keep YAML.
const JSONConfigMinVersion = "v0.15.5-rc.1"

// maxJSONInteger is the largest integer every JSON implementation reads
// exactly, 2^53-1.
const maxJSONInteger = 1<<53 - 1

var (
	jsonIntegerPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)
	// decimalPattern is how a JSON config writes a fractional quantity: as a
	// string, because its numbers are integers only.
	decimalPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?$`)
)

// RequiresJSON reports whether a config that pins cvmVersion must be JSON.
func RequiresJSON(cvmVersion string) bool {
	version, _, _ := strings.Cut(cvmVersion, "@sha256:")
	return semver.Compare("v"+strings.TrimPrefix(version, "v"), JSONConfigMinVersion) >= 0
}

// CheckJSONForm requires the one byte form a JSON config has: RFC 8785
// canonical JSON whose numbers are integers that every implementation reads
// exactly, without null. Each config then has a single digest, and no two
// parsers can read its bytes differently. It does not check the config's
// content; Decode does both.
func CheckJSONForm(data []byte) error {
	decoder := jsontext.NewDecoder(bytes.NewReader(data))
	for {
		switch decoder.PeekKind() {
		case 'n':
			return fmt.Errorf("null is not allowed; leave the member out")
		case '0':
			number, err := decoder.ReadValue()
			if err != nil {
				return err
			}
			integer, err := strconv.ParseInt(string(number), 10, 64)
			if !jsonIntegerPattern.Match(number) || err != nil || integer > maxJSONInteger || integer < -maxJSONInteger {
				return fmt.Errorf("number %s is not an integer between -(2^53-1) and 2^53-1; write other quantities as strings", number)
			}
			continue
		}
		if _, err := decoder.ReadToken(); err == io.EOF {
			break
		} else if err != nil {
			return err
		}
	}
	canonical := jsontext.Value(bytes.Clone(data))
	if err := canonical.Canonicalize(); err != nil {
		return err
	}
	if !bytes.Equal(canonical, data) {
		return fmt.Errorf("config is not in canonical form (RFC 8785)")
	}
	return nil
}

// jsonUnmarshalers read the types a JSON config writes as strings.
var jsonUnmarshalers = json.JoinUnmarshalers(
	json.UnmarshalFromFunc(func(decoder *jsontext.Decoder, value *float64) error {
		text, err := readJSONString(decoder)
		if err != nil {
			return err
		}
		if !decimalPattern.MatchString(text) {
			return fmt.Errorf("%q is not a plain decimal", text)
		}
		*value, err = strconv.ParseFloat(text, 64)
		return err
	}),
	json.UnmarshalFromFunc(func(decoder *jsontext.Decoder, value *time.Duration) error {
		text, err := readJSONString(decoder)
		if err != nil {
			return err
		}
		*value, err = time.ParseDuration(text)
		return err
	}),
	// A field without a fixed type holds an integer or a string. Reading it
	// here keeps the decoder from routing its numbers through float64.
	json.UnmarshalFromFunc(func(decoder *jsontext.Decoder, value *any) (err error) {
		*value, err = readJSONUntyped(decoder)
		return err
	}),
)

func readJSONUntyped(decoder *jsontext.Decoder) (any, error) {
	switch decoder.PeekKind() {
	case '0':
		number, err := decoder.ReadValue()
		if err != nil {
			return nil, err
		}
		return strconv.Atoi(string(number))
	case '{':
		if _, err := decoder.ReadToken(); err != nil {
			return nil, err
		}
		members := map[string]any{}
		for decoder.PeekKind() != '}' {
			name, err := decoder.ReadToken()
			if err != nil {
				return nil, err
			}
			if members[name.String()], err = readJSONUntyped(decoder); err != nil {
				return nil, err
			}
		}
		_, err := decoder.ReadToken()
		return members, err
	}
	token, err := decoder.ReadToken()
	if err != nil {
		return nil, err
	}
	switch token.Kind() {
	case '"':
		return token.String(), nil
	case 't', 'f':
		return token.Bool(), nil
	}
	return nil, fmt.Errorf("must be an integer, a string or an object")
}

func readJSONString(decoder *jsontext.Decoder) (string, error) {
	token, err := decoder.ReadToken()
	if err != nil {
		return "", err
	}
	if token.Kind() != '"' {
		return "", fmt.Errorf("must be a string")
	}
	return token.String(), nil
}

func unmarshalJSON(data []byte, target any) error {
	return json.Unmarshal(data, target, json.RejectUnknownMembers(true), json.WithUnmarshalers(jsonUnmarshalers))
}

func decodeShimJSON(data jsontext.Value) (*ShimConfig, error) {
	var config ShimConfig
	if err := defaults.Set(&config); err != nil {
		return nil, fmt.Errorf("failed to set defaults: %v", err)
	}
	if len(data) > 0 {
		if err := unmarshalJSON(data, &config); err != nil {
			return nil, fmt.Errorf("failed to decode config: %v", err)
		}
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &config, nil
}

// UnmarshalJSON reads the repository ID as the decimal string a JSON config
// writes it as.
func (g *GitHubProvenance) UnmarshalJSON(data []byte) error {
	var raw struct {
		Repository   string `json:"repository"`
		RepositoryID string `json:"repository_id"`
		Workflow     string `json:"workflow"`
		Ref          string `json:"ref"`
	}
	if err := json.Unmarshal(data, &raw, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	if !repositoryIDPattern.MatchString(raw.RepositoryID) {
		return fmt.Errorf("provenance.github.repository_id must be the repository's numeric ID in plain decimal")
	}
	id, err := strconv.ParseInt(raw.RepositoryID, 10, 64)
	if err != nil {
		return err
	}
	*g = GitHubProvenance{Repository: raw.Repository, RepositoryID: id, Workflow: raw.Workflow, Ref: raw.Ref}
	return nil
}
