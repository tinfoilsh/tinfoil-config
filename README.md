# tinfoil-config

Canonical Go types and validation for the measured Tinfoil workload YAML format.

This repository owns the current v1 YAML contract shared by `cvmimage`,
`tinfoild`, and `measure-image-action`. It deliberately does not own launcher
metadata or perform translation to a future runtime format.

## Go API

```go
cfg, err := tinfoilconfig.Decode(data, tinfoilconfig.Options{})
```

Use `HostDebugMode` only after trusted `tinfoild` debug injection. It permits
the reserved debug toolbox's two runtime socket mounts; all other validation
still applies.

## CLI

```sh
go run ./cmd/tinfoil-config config.yml
```

The command exits non-zero and prints the exact schema or policy violation for
invalid input.

Plaintext models without a `containers[].models` grant retain the legacy shared
mount behavior. Granted models use isolated named mounts; encrypted models
always require at least one explicit container grant.

## CVM administration

`containers[].cvm_admin: true` opts into a fixed CVM-wide administrative profile:
UID/GID 0, privileged container execution, the host PID namespace,
no-new-privileges disabled, the CVM filesystem at `/host`, and the Docker socket
at `/var/run/docker.sock`. The container rootfs defaults to writable; an explicit
`read_only: true` is still honored. Networking is unchanged: declare bridge
`networks` and `ports` as usual. For example, an `egress: open` network and
`ports: ["2022:2222"]` provide Internet access and SSH over the shim's attested
CONNECT tunnel, without host networking or direct SSH ingress. Containers
launched through Docker must join the declared network to use its egress policy.

The flag defaults to false and is part of the measured YAML. It does not enable
debug mode or change keyserver/attestation policy. Anyone controlling an admin
container must be trusted with the entire CVM, including other workloads and
their secrets. Guest network policy cannot constrain that administrator.

Declared volumes retain their existing persistence/unlock semantics. The
container's writable layer and Docker state remain ephemeral across CVM reboot;
use an attached volume for durable data. For Docker bind mounts, expose the
volume at its CVM path, `/run/tinfoil/volumedata/<name>`, inside the admin container
too. A different container-only alias is not a Docker host path.

This requires a CVM image implementing the profile and updated config validators;
older images reject the new field. Legacy debug-toolbox injection remains
supported through `HostDebugMode`.

## CVM source

`cvm-source` names the GitHub repository whose release carries the image
manifest and attestation, and the https base URL that serves its kernel,
initrd, and raw disk:

```yaml
cvm-source:
  repo: tinfoilsh/cvmimage-sandbox
  artifacts: https://images.example.com/cvm
```

Both fields are required when the block is present. Without it,
`tinfoilsh/cvmimage` and `https://images.tinfoil.sh/cvm` apply.
