package tinfoilconfig

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	ReservedDebugContainerName = "tinfoil-debug-toolbox"
	ReservedDebugPort          = "2222/tcp"
	ReservedDebugHostPort      = 2222
	AttestedKeysContainerDir   = "/run/tinfoil/keys"
	KeyECDSAP256               = "ecdsa-p256"
	KeyEd25519                 = "ed25519"
	KeyX25519                  = "x25519"
)

// ValidationMode selects which producer is allowed to construct the config.
// WorkloadMode is for a user-supplied measured config. HostDebugMode is only
// for a config after tinfoild has injected its reserved debug toolbox.
type ValidationMode uint8

const (
	WorkloadMode ValidationMode = iota
	HostDebugMode
)

type Options struct {
	Mode ValidationMode
}

type Config struct {
	CVMVersion   string                  `yaml:"cvm-version" json:"cvm-version"`
	ShimCfg      *ShimConfig             `yaml:"shim" json:"-"`
	CVMNetwork   CVMNetworkConfig        `yaml:"cvm-network" json:"cvm-network"`
	Networks     map[string]*NetworkSpec `yaml:"networks" json:"networks"`
	CPUs         int                     `yaml:"cpus" json:"cpus"`
	Memory       int                     `yaml:"memory" json:"memory"`
	GPUs         int                     `yaml:"gpus" json:"gpus"`
	Security     SecurityConfig          `yaml:"security,omitempty" json:"security"`
	Models       []ModelSpec             `yaml:"models" json:"models"`
	Volumes      []VolumeSpec            `yaml:"volumes" json:"volumes"`
	AttestedKeys []AttestedKey           `yaml:"attested-keys,omitempty" json:"attested-keys"`
	Containers   []Container             `yaml:"containers" json:"containers"`
	KeyserverURL string                  `yaml:"keyserver-url,omitempty" json:"keyserver-url"`
}

type SecurityConfig struct {
	AllowUnencryptedNVLink     bool `yaml:"allow-unencrypted-nvlink,omitempty" json:"allow-unencrypted-nvlink"`
	IncludeConfigInAttestation bool `yaml:"include-config-in-attestation,omitempty" json:"include-config-in-attestation"`
}

type CVMNetworkConfig struct {
	InboundPorts []int `yaml:"inbound-ports" json:"inbound-ports"`
}

type NetworkSpec struct {
	Egress string   `yaml:"egress" json:"egress"`
	Allow  []string `yaml:"allow" json:"allow"`
}

type VolumeSpec struct {
	Name      string          `yaml:"name" json:"name"`
	Exec      bool            `yaml:"exec,omitempty" json:"exec"`
	Owner     string          `yaml:"owner,omitempty" json:"owner"`
	KeySecret string          `yaml:"key-secret,omitempty" json:"key-secret"`
	Overlays  []VolumeOverlay `yaml:"overlays,omitempty" json:"overlays"`
	// Size is the capacity of the disk backing the volume, such as "500GiB"
	// or "16TB" (see ParseSize). Empty leaves the size to the host's default.
	// The host allocates the disk once, on first launch; changing the size
	// afterwards does not resize it.
	Size string `yaml:"size,omitempty" json:"size"`
}

// SizeBytes returns the declared Size in bytes, or 0 when the host default
// applies.
func (v *VolumeSpec) SizeBytes() (int64, error) {
	if v.Size == "" {
		return 0, nil
	}
	return ParseSize(v.Size)
}

// OwnerIDs returns the uid and gid declared as Owner "uid:gid" or "id" for
// both, or root when Owner is empty.
func (v *VolumeSpec) OwnerIDs() (uid, gid int, err error) {
	if v.Owner == "" {
		return 0, 0, nil
	}
	uidText, gidText, found := strings.Cut(v.Owner, ":")
	if !found {
		gidText = uidText
	}
	uid, uidErr := strconv.Atoi(uidText)
	gid, gidErr := strconv.Atoi(gidText)
	if uidErr != nil || gidErr != nil || uid < 0 || uid > maxVolumeOwner || gid < 0 || gid > maxVolumeOwner {
		return 0, 0, fmt.Errorf("owner %q must be id or uid:gid, each between 0 and %d", v.Owner, maxVolumeOwner)
	}
	return uid, gid, nil
}

// VolumeOverlay stacks a model pack under a writable directory on the volume:
// Source is a subtree of the Model pack, taken as the read-only lower layer,
// and the merged tree appears at Target inside the volume. Both paths are
// measured here rather than chosen at unlock, so no runtime caller can steer
// the mount.
type VolumeOverlay struct {
	Model  string `yaml:"model" json:"model"`
	Source string `yaml:"source" json:"source"`
	Target string `yaml:"target" json:"target"`
}

type ModelSpec struct {
	Name      string `yaml:"name,omitempty" json:"name"`
	Repo      string `yaml:"repo,omitempty" json:"repo"`
	MPK       string `yaml:"mpk,omitempty" json:"mpk"`
	MWP       string `yaml:"mwp,omitempty" json:"mwp"`
	EMWP      string `yaml:"emwp,omitempty" json:"emwp"`
	KeySecret string `yaml:"key-secret,omitempty" json:"key-secret"`
	// Weights are never programs, so a pack is noexec unless declared otherwise.
	Exec bool `yaml:"exec,omitempty" json:"exec"`
	// Schema is the pack schema the pinned artifact was built with (0 = schema 1,
	// the original layout). Declarative for now; no consumer reads it yet.
	Schema int `yaml:"schema,omitempty" json:"schema"`
}

// AttestedKey declares a boot-generated key. The runtime exports PKCS#8 private
// and SPKI public PEM, and endorses full public SPKI DER in v3 attestation.
// UID and GID are measured numeric file ownership, defaulting to root.
type AttestedKey struct {
	ID  string `yaml:"id" json:"id"`
	Key string `yaml:"key" json:"key"`
	UID int    `yaml:"uid,omitempty" json:"uid"`
	GID int    `yaml:"gid,omitempty" json:"gid"`
}

type Container struct {
	Name              string            `yaml:"name" json:"name"`
	Image             string            `yaml:"image" json:"image"`
	Provenance        *Provenance       `yaml:"provenance,omitempty" json:"provenance"`
	CVMAdmin          bool              `yaml:"cvm_admin,omitempty" json:"cvm_admin"` // Delegates administration of the entire CVM.
	SealRegister      bool              `yaml:"seal_register,omitempty" json:"seal_register"`
	Attestation       bool              `yaml:"attestation,omitempty" json:"attestation"`
	Command           []string          `yaml:"command,omitempty" json:"command"`
	Entrypoint        []string          `yaml:"entrypoint,omitempty" json:"entrypoint"`
	WorkingDir        string            `yaml:"working_dir,omitempty" json:"working_dir"`
	User              string            `yaml:"user,omitempty" json:"user"`
	Env               []interface{}     `yaml:"env,omitempty" json:"env"`
	Secrets           []string          `yaml:"secrets,omitempty" json:"secrets"`
	Models            []string          `yaml:"models,omitempty" json:"models"`
	Keys              []string          `yaml:"keys,omitempty" json:"keys"`
	Volumes           []string          `yaml:"volumes,omitempty" json:"volumes"`
	PersistentVolumes []string          `yaml:"persistent_volumes,omitempty" json:"persistent_volumes"`
	Devices           []string          `yaml:"devices,omitempty" json:"devices"`
	CapAdd            []string          `yaml:"cap_add,omitempty" json:"cap_add"`
	Runtime           string            `yaml:"runtime,omitempty" json:"runtime"`
	Networks          []string          `yaml:"networks,omitempty" json:"networks"`
	Ports             []string          `yaml:"ports,omitempty" json:"ports"`
	IPC               string            `yaml:"ipc,omitempty" json:"ipc"`
	PidMode           string            `yaml:"pid,omitempty" json:"pid"`
	GPUs              interface{}       `yaml:"gpus,omitempty" json:"gpus"`
	ShmSize           string            `yaml:"shm_size,omitempty" json:"shm_size"`
	Memory            string            `yaml:"memory,omitempty" json:"memory"`
	CPUs              float64           `yaml:"cpus,omitempty" json:"cpus"`
	Tmpfs             map[string]string `yaml:"tmpfs,omitempty" json:"tmpfs"`
	ReadOnly          *bool             `yaml:"read_only,omitempty" json:"read_only"`
	PidsLimit         *int64            `yaml:"pids_limit,omitempty" json:"pids_limit"`
	Restart           string            `yaml:"restart,omitempty" json:"restart"`
	StopSignal        string            `yaml:"stop_signal,omitempty" json:"stop_signal"`
	StopTimeout       *int              `yaml:"stop_timeout,omitempty" json:"stop_timeout"`
	Healthcheck       *Healthcheck      `yaml:"healthcheck,omitempty" json:"healthcheck"`
}

// Provenance names the build that must have produced the container image.
// Verifiers reject a deployment whose attestation lacks a matching proof.
type Provenance struct {
	GitHub *GitHubProvenance `yaml:"github" json:"github"`
}

// GitHubProvenance identifies a GitHub Actions workflow run. Ref is the exact
// tag or branch ref it ran for, and Commit the source commit it built.
type GitHubProvenance struct {
	Repository   string `yaml:"repository" json:"repository"`
	RepositoryID int64  `yaml:"repository_id" json:"repository_id"`
	Workflow     string `yaml:"workflow" json:"workflow"`
	Ref          string `yaml:"ref" json:"ref"`
	Commit       string `yaml:"commit" json:"commit"`
}

type Healthcheck struct {
	Test        []string `yaml:"test" json:"test"`
	Interval    string   `yaml:"interval,omitempty" json:"interval"`
	Timeout     string   `yaml:"timeout,omitempty" json:"timeout"`
	Retries     int      `yaml:"retries,omitempty" json:"retries"`
	StartPeriod string   `yaml:"start_period,omitempty" json:"start_period"`
}
