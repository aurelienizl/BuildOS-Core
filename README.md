# BuildOS Core

BuildOS Core compiles ordered YAML configurations into executable Bash scripts for Debian and Ubuntu. Generate and review a script, then run it as root on the target machine.

Supported operations: APT packages, systemd services, sysctl configuration, local and remote files, inline text and binary files, users, snap artifacts, Docker image imports, and shell commands.

See [DOCUMENTATION.md](DOCUMENTATION.md) for the CLI, configuration format, module reference, and execution constraints.

## Quick start

Requires Make and an accessible Docker or Podman engine. Build images provide Go; no host Go installation is required.

```bash
make build
./buildos --config config.yaml --output setup.sh
bash -n setup.sh
less setup.sh
sudo ./setup.sh
```

Example `config.yaml`:

```yaml
version: 1
base:
  distribution: debian
  release: '12'
packages:
  install: [curl, ca-certificates]
files_text:
  - destination: /etc/buildos-release
    content: 'Managed by BuildOS'
    mode: '644'
```

The generated script targets Linux and requires Bash and root. The binary targets the build container's platform. Script generation does not execute target operations. Generated scripts embed resolved secrets; treat them as deployment credentials.

## Development

| Command | Result |
| --- | --- |
| `make build` | Check formatting, run `go vet`, compile the CLI and integration tests; export `./buildos` |
| `make fmt` | Format `src/` and `tests/` in place |
| `make test` | Run module profiles and regression tests on Debian 12 and Ubuntu 24.04 |
| `make test TARGET=debian:12-slim` | Test one distribution |
| `make test TARGET=ubuntu:24.04 MODULE=files_text` | Test one module profile; skip regressions |
| `make coverage` | Run tests and write HTML, function totals, and Go coverage data under `coverage/` |

Append `CONTAINER_ENGINE=podman` to any Make command to use Podman. `make test-containers` aliases `make test`; the runner can also be invoked with `bash tests/containers.sh`.

Builds require network access for images and dependencies; package tests also access APT repositories. Tests execute system changes in fresh disposable containers without privileged mode or host mounts. Do not bypass the direct `go test` execution guard. Snap and Docker tests use command doubles; they do not validate live daemons. Runtime service operations also use doubles; persistent systemd enable/mask settings are tested directly.

Coverage measures Go statements in the generator, not generated Bash. `TARGET`, `MODULE`, and `CONTAINER_ENGINE` also apply to coverage; each run replaces previous data. Reports are published after all selected tests pass.

The [VS Code devcontainer](.devcontainer/devcontainer.json) provides Go, Make, and Docker-in-Docker. It runs as root and uses privileged mode for its dedicated Docker daemon.

## Source layout and contracts

| Location | Responsibility |
| --- | --- |
| `src/cmd/buildos` | CLI, input overwrite protection, atomic script publication |
| `src/config` | Ordered YAML loading, includes, strict decoding, `.env` interpolation |
| `src/script` | Module registry and generation orchestration |
| `src/modules` | Module validation and shell emission |
| `src/ops` | Bash startup, argument quoting, file operations, downloads, logging |
| `src/util` | Shared validation |
| `tests` | Module profiles, regression fixtures, container runner |

Preserve these contracts when contributing:

- Keep module occurrences and includes ordered. A file's modules precede its included files; nested includes finish before the next sibling.
- Validate the complete configuration before publishing output. Failed generation preserves existing output; successful publication is atomic with mode `0700` and protects loaded inputs from overwrite.
- Keep target operations in the generated script. Pass structured arguments through `ops.Run`; use explicit shell emitters for command strings.
- Load one `.env` beside the main config. Interpolate string values once, without process-environment fallback; suppress value-bearing validation errors in interpolated modules.
- Preserve runtime root/distribution checks, failure exit status, and temporary-download cleanup. Completed operations are not rolled back.

To add a module, define its YAML type in `src/config/config.go`, implement and register its emitter in `src/script/script.go`, add an isolated profile and verifier in `tests/`, and document its fields. Run formatting, build, and the full test suite after changing shared behavior.

Keep deployment profiles, credentials, assets, and generated scripts in a separate private repository. Root `profile/`, `.env`, `buildos`, `script.sh`, and coverage output are ignored and excluded from Docker build contexts; custom output names need their own ignore rules. Synthetic fixtures under `tests/profile/` belong in this repository.

Licensed under [MIT](LICENSE).
