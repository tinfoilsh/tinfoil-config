package tinfoilconfig

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
)

const (
	MaxAttestedKeys          = 32
	AttestedKeysContainerDir = "/run/tinfoil/keys"
	// MinCVMVersionAttestedKeys is the minimum official CVM version for
	// attested keys and direct admin SSH.
	MinCVMVersionAttestedKeys = "0.15.0"
	KeyECDSAP256              = "ecdsa-p256"
	KeyEd25519                = "ed25519"
	KeyX25519                 = "x25519"
)

// AttestedKey declares a boot-generated key. The runtime exports PKCS#8 private
// and SPKI public PEM, and endorses full public SPKI DER in v3 attestation.
// UID and GID are measured numeric file ownership, defaulting to root.
type AttestedKey struct {
	ID  string `yaml:"id" json:"id"`
	Key string `yaml:"key" json:"key"`
	UID int    `yaml:"uid,omitempty" json:"uid"`
	GID int    `yaml:"gid,omitempty" json:"gid"`
}

var attestedKeyIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// ValidateAttestedKeys checks declarations, exclusive grants, ownership and
// generated mount destinations. It is also used by the boot key store before
// deriving filesystem paths from a config.
func ValidateAttestedKeys(config *Config) error {
	if config == nil {
		return fmt.Errorf("missing config")
	}
	if len(config.AttestedKeys) > MaxAttestedKeys {
		return fmt.Errorf("attested-keys exceeds limit %d", MaxAttestedKeys)
	}
	if len(config.AttestedKeys) > 0 {
		if err := requireAttestedKeysRuntime(config); err != nil {
			return err
		}
	}
	declared := make(map[string]bool, len(config.AttestedKeys))
	for i, key := range config.AttestedKeys {
		if !attestedKeyIDPattern.MatchString(key.ID) || key.ID == "tls" || key.ID == "hpke" {
			return fmt.Errorf("attested-keys[%d].id %q must be a safe lowercase name other than tls or hpke", i, key.ID)
		}
		if declared[key.ID] {
			return fmt.Errorf("attested-keys id %q is declared twice", key.ID)
		}
		declared[key.ID] = true
		switch key.Key {
		case KeyECDSAP256, KeyEd25519, KeyX25519:
		default:
			return fmt.Errorf("attested-keys[%d].key %q is unsupported", i, key.Key)
		}
		if key.UID < 0 || key.UID > 65534 || key.GID < 0 || key.GID > 65534 {
			return fmt.Errorf("attested-keys[%d] uid and gid must be between 0 and 65534", i)
		}
	}
	granted := make(map[string]string, len(declared))
	for _, container := range config.Containers {
		for _, id := range container.Keys {
			if !declared[id] {
				return fmt.Errorf("container %q grants unknown attested key %q", container.Name, id)
			}
			if prior, ok := granted[id]; ok {
				return fmt.Errorf("attested key %q is already granted to %q (duplicate or shared grant)", id, prior)
			}
			granted[id] = container.Name
		}
		if len(container.Keys) == 0 {
			continue
		}
		// Protect generated destinations in containers receiving keys.
		// Parent tmpfs /run is supported: Docker mounts nested key binds after it.
		for target := range container.Tmpfs {
			if overlapsKeyDir(target) && path.Clean(target) != "/run" {
				return fmt.Errorf("container %q tmpfs %q conflicts with attested key mounts", container.Name, target)
			}
		}
		for _, volume := range container.Volumes {
			_, target, ok := strings.Cut(volume, ":")
			if ok && overlapsKeyDir(target) {
				return fmt.Errorf("container %q volume target %q conflicts with attested key mounts", container.Name, target)
			}
		}
	}
	for _, key := range config.AttestedKeys {
		if _, ok := granted[key.ID]; !ok {
			return fmt.Errorf("attested key %q requires exactly one container grant", key.ID)
		}
	}
	return nil
}

func overlapsKeyDir(target string) bool {
	target = path.Clean(target)
	return target == "/" || target == AttestedKeysContainerDir ||
		strings.HasPrefix(target, AttestedKeysContainerDir+"/") ||
		strings.HasPrefix(AttestedKeysContainerDir, target+"/")
}

func requireAttestedKeysRuntime(config *Config) error {
	if config.CVMSource.OrDefault() != DefaultCVMSource {
		// A measured custom runtime has its own version namespace. Its publisher
		// must implement this contract; official version numbers cannot prove it.
		return nil
	}
	version := strings.TrimPrefix(config.CVMVersion, "v")
	version, _, _ = strings.Cut(version, "+")
	parts := strings.Split(version, ".")
	var numbers [3]int
	valid := len(parts) == 3
	if valid {
		for i, part := range parts {
			n, err := strconv.Atoi(part)
			if err != nil || n < 0 || strconv.Itoa(n) != part {
				valid = false
				break
			}
			numbers[i] = n
		}
	}
	if !valid || (numbers[0] == 0 && numbers[1] < 15) {
		return fmt.Errorf("attested keys and direct admin SSH require official cvm-version >= %s (got %q)", MinCVMVersionAttestedKeys, config.CVMVersion)
	}
	return nil
}
