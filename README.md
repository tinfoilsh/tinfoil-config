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
