# BuildOS Core — configuration and operations

BuildOS reads local YAML and generates a Bash script. System changes and downloads happen only when that script runs on the target. For builds, tests, and contribution guidelines, see [README.md](README.md).

## Generate and apply

```bash
./buildos --config config.yaml --output setup.sh
bash -n setup.sh
less setup.sh
sudo ./setup.sh
```

| CLI option | Contract |
| --- | --- |
| `--config` | Required main YAML path |
| `--output` | Required output path; parent directory must exist |
| `--help` | Show usage |

CLI paths are relative to the generator's working directory unless absolute. Positional arguments and `--dry-run` are unsupported. Generation needs readable inputs and a writable output directory, without root or target tools. Use a binary matching the generator machine's OS and architecture.

Successful generation atomically replaces the output with mode `0700`. Failures preserve existing output. Output cannot overwrite loaded YAML files or `.env`, including existing symlink and hard-link aliases. Generation validates configuration, not target commands, repositories, assets, or checksum contents. `bash -n` checks shell syntax only.

For remote deployment, transfer the script and any external local assets. The target does not need the generator, YAML, or `.env`. Inline content and resolved secrets are embedded in the script; preserve its restrictive permissions.

## Target requirements

Every script requires Bash, root, `/etc/os-release` identifying `debian` or `ubuntu`, and standard system utilities including `mktemp` and `rm`. The integration test matrix covers Debian 12 and Ubuntu 24.04; other releases are not covered by that matrix.

| Operation | Additional requirements |
| --- | --- |
| Packages | `apt-get`, repository access; `dpkg-query` for purge |
| Services | `systemctl`; running systemd for runtime actions |
| Files | `mkdir`, `chown`, `chmod`; `cp` and `sha256sum` for source files |
| HTTP/HTTPS assets | `wget`, network access, certificate trust for HTTPS |
| Users | `id`, `useradd`, `usermod`; existing groups and executable shell |
| Snaps | `snap` and running snapd |
| Containers | Docker CLI, running Docker daemon, `sha256sum` |
| Shell commands | All commands invoked by the configuration |

Prerequisites are not installed automatically. Install them in an earlier module or prepare the target beforehand.

## Configuration and ordering

Use one YAML mapping, optionally wrapped entirely in `buildos:`:

```yaml
buildos:
  version: 1
  base:
    distribution: ubuntu
    release: '24.04'
  packages:
    install: [nginx]
  files_text:
    - destination: /var/www/html/index.html
      content: |
        <!doctype html>
        <h1>Managed by BuildOS</h1>
  exec_cmd:
    - nginx -t
  services:
    enable: [nginx.service]
    start: [nginx.service]
```

`version` is optional; integer `0` and `1` are accepted. Use `1` for new configurations. If present, `buildos` must be the only root key. Unknown modules/fields, duplicate entry fields, repeated `version`, multiple documents, and null modules fail generation. Module-level YAML aliases are unsupported. Use `{}` for an empty mapping module and `[]` for an empty list module.

Modules run in written order. Repeated module keys are intentional and every occurrence runs; generic YAML tools may reject or collapse them. List entries preserve their order. `packages` and `services` use fixed internal action orders regardless of field order.

### Includes

```yaml
base:
  distribution: debian
  release: '12'
  include: [fragments/packages.yaml, fragments/services.yaml]
exec_cmd:
  - echo 'Main configuration'
```

Execution order: all modules in this file, then `packages.yaml` and its descendants, then `services.yaml` and its descendants. An include is processed after its containing file's modules, regardless of its `base` position.

Paths must be non-empty and relative to the containing YAML path, including when that path is a symlink. Repeated includes execute repeatedly; missing files and active include cycles fail generation. Each fragment accepts the same document format. Includes from repeated `base` entries are collected in appearance order.

## Secrets

Place one optional `.env` beside the main `--config` path. All fragments share it; fragment-specific `.env` files and exported process variables are ignored. When the main path is a symlink, discovery uses the symlink's directory.

```dotenv
ADMIN_PASSWORD_HASH='$6$salt$replace-with-a-real-hash'
APP_PASSWORD='literal-$-and-#-characters'
```

```yaml
users:
  - username: administrator
    password_hash: '${env:ADMIN_PASSWORD_HASH}'
files_text:
  - destination: /etc/myapp/credentials.conf
    mode: '600'
    content: |
      password=${env:APP_PASSWORD}
```

Use `chmod 600 .env`. An existing file is always validated and must resolve to a readable regular file. Broken symlinks fail generation.

| Syntax | Behavior |
| --- | --- |
| `NAME=value` | One literal assignment per line; LF/CRLF accepted |
| `NAME='value'` or `NAME="value"` | Remove matching outer quotes; preserve their contents |
| `NAME=` | Empty string |
| Blank line or full-line `#` comment | Ignored |
| `${env:NAME}` in a YAML string | Substitute once during generation |
| `$${env:NAME}` | Emit literal `${env:NAME}` without lookup |

Names match `[A-Za-z_][A-Za-z0-9_]*`. Surrounding assignment whitespace is trimmed. There is no `export`, multiline syntax, escape decoding, inline-comment handling, or shell evaluation in `.env`. Duplicate names, invalid assignments, NUL bytes, and unmatched quotes fail generation.

Substitution affects string values, including include paths; keys and YAML types stay unchanged. Missing variables, malformed placeholders, and default expressions such as `${env:NAME:-fallback}` fail generation. Inserted values are never expanded again. Errors in interpolated modules omit value-bearing validation details.

Resolved secrets remain in the script. Command logs omit arguments, but paths and command output may disclose sensitive data. In `exec_cmd` and `exec`, substitution inserts Bash source: quoting remains the configuration author's responsibility.

## Module reference

### `base` — mapping

| Field | Type | Behavior |
| --- | --- | --- |
| `distribution` | String | `debian` or `ubuntu`; requires `release`; trimmed and lowercased |
| `release` | String | Exact target `VERSION_ID`; requires `distribution`; trimmed |
| `include` | String list | Relative YAML paths; default empty |

Always quote releases, such as `'12'` or `'24.04'`. Every script checks the distribution family at startup. Exact distribution/release assertions run at their module position; put the assertion first in the main file to prevent earlier changes on a mismatched release. `base: {}` and include-only entries are valid.

### `exec_cmd` — string list

```yaml
exec_cmd:
  - |
    cd /opt/myapp
    ./configure.sh
    ./verify.sh
```

Each non-empty entry runs as root in a separate `bash -Eeuo pipefail -c` process. Working-directory and environment changes do not persist between entries. Group dependent commands in one multiline entry. Explicit shell error handling (`if`, `||`, etc.) follows Bash semantics. Scripts export `DEBIAN_FRONTEND=noninteractive`. Use this module for commands formerly expressed as `hooks`.

### `packages` — mapping

| Field | Type | Default / action |
| --- | --- | --- |
| `extra_args` | String list | Empty; literal arguments before every APT operation in this occurrence |
| `install` | String list | Empty; `install -y` |
| `remove` | String list | Empty; `remove -y` |
| `purge` | String list | Empty; purge installed packages or residual configuration; skip absent packages |
| `upgrade` | Boolean | `false`; `upgrade -y` |
| `autoremove` | Boolean | `false`; `autoremove -y` |
| `clean` | Boolean | `false`; `clean` |

Order: **update → install → remove → purge → upgrade → autoremove → clean**. Update runs only when installing or upgrading.

```yaml
packages:
  extra_args: ['-o', 'Acquire::Retries=3', '-o', 'APT::Update::Error-Mode=any']
  install: [curl, ca-certificates]
  autoremove: true
  clean: true
```

Each extra argument is one literal token, without shell expansion or word splitting. For a proxy, use `['-o', 'Acquire::http::Proxy=http://proxy:3142']`; there is no `proxy` field. Options do not carry into later module occurrences.

### `services` — mapping

All fields are optional lists of unit names, default empty. Fixed action order:

**mask → unmask → enable → disable → start → stop → restart → reload**.

```yaml
services:
  unmask: [nginx.service]
  enable: [nginx.service]
  start: [nginx.service]
```

Enable/disable affect boot configuration; start/stop affect runtime state. Reload requires unit support. Repeat the module to express other sequences. After writing systemd unit files, run `systemctl daemon-reload` explicitly through `exec_cmd`.

### `kernel` — mapping

```yaml
kernel:
  sysctl:
    net.ipv4.ip_forward: 1
    vm.swappiness: 10
exec_cmd:
  - sysctl -p /etc/sysctl.d/99-buildos.conf
```

`sysctl` is an optional parameter/value mapping. Use single-line scalar values. Keys accept letters, digits, underscores, dots, and hyphens; support for actual parameters and values is determined by the target kernel.

A non-empty mapping replaces `/etc/sysctl.d/99-buildos.conf`, with settings sorted by key. Occurrences replace rather than merge; an empty mapping does nothing. Writing persistent configuration does not alter the running kernel. The following `exec_cmd` applies it immediately; otherwise, the target's boot-time sysctl loader can apply it.

### File modules — entry lists

`files`, `files_text`, and `files_base64` share these fields:

| Field | Type | Required / default |
| --- | --- | --- |
| `name` | String | Optional diagnostic label |
| `destination` | String | Required absolute target file path |
| `owner` | String | `root`; must exist |
| `group` | String | `root`; must exist |
| `mode` | String or integer | `'644'`; prefer quoted octal digits, e.g. `'600'` or `'0755'` |
| `exec` | String | Optional post-write Bash command, run in the destination's parent directory |

Modes accept three or four digits from `0` to `7`. Parent directories are created automatically. Files are overwritten, then ownership and mode are applied, then non-blank `exec` runs. Writes are in place, not atomic; an interrupted write can leave partial content. Existing destination symlinks are followed. Permissions are finalized after writing; see [execution constraints](#execution-constraints).

**`files`:** requires non-empty `source` (target-local path or HTTP/HTTPS URL) and `checksum` (`sha256:` plus exactly 64 hexadecimal digits).

```yaml
files:
  - source: /opt/assets/myapp.conf
    destination: /etc/myapp/myapp.conf
    checksum: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    mode: '640'
    exec: systemctl reload myapp.service
```

Replace the sample digest with `sha256sum` of the actual source. Execution checks bytes before copying. Local sources are preserved; downloaded sources are temporary.

**`files_text`:** requires string `content`; missing/null content is invalid. Writes resolved text literally, preserving quotes, `$HOME`, `$(...)`, percent signs, and UTF-8 without shell evaluation.

```yaml
files_text:
  - destination: /etc/myapp/myapp.conf
    content: |
      [server]
      listen = 8080
```

`content: ''` writes an empty file. YAML `|` retains one trailing newline, `|-` strips it, and `|+` preserves trailing blank lines. A quoted single-line value has no implicit newline.

**`files_base64`:** requires string `content` in standard base64 with correct padding where needed. Decoded during generation; no target decoder is required. CR/LF are accepted; other whitespace and URL-safe base64 are rejected. Exact bytes, including NUL and trailing newlines, are preserved.

```yaml
files_base64:
  - destination: /etc/myapp/greeting.txt
    content: aGVsbG8K
```

This writes `hello` and a newline. Encode assets with GNU `base64 -w 0 file.bin`; `content: ''` writes an empty file. Inline modules have no `source` or `checksum` fields. Base64 provides no encryption.

### `snaps` — entry list

| Field | Required / behavior |
| --- | --- |
| `name` | Optional label |
| `assert` | Required non-empty target-local path or HTTP/HTTPS URL |
| `snap` | Required non-empty target-local path or HTTP/HTTPS URL |

```yaml
snaps:
  - name: application
    assert: /opt/assets/application.assert
    snap: /opt/assets/application.snap
```

Runs `snap ack`, then `snap install` for each entry. No Store-name lookup, checksum, channel, confinement options, or post-install command is provided. Use a following `exec_cmd` for additional actions.

### `containers` — entry list

| Field | Required / behavior |
| --- | --- |
| `name` | Required non-empty label; does not set a tag or container name |
| `source` | Required non-empty target-local archive path or HTTP/HTTPS URL |
| `checksum` | Required SHA-256 string, same format as `files` |
| `exec` | Optional host Bash command after image load |

```yaml
containers:
  - name: application
    source: /opt/assets/application.tar.gz
    checksum: sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
```

Prepare an archive with `docker image save application:latest | gzip > application.tar.gz` and replace the sample checksum. Execution verifies it and runs `docker load -i`. Running containers requires an explicit `exec` or later `exec_cmd`; repeat-safe lifecycle management is the author's responsibility. A foreground command blocks subsequent modules.

### `users` — entry list

| Field | Type | Required / default |
| --- | --- | --- |
| `username` | String | Required; matches `^[a-z_][a-z0-9_-]*[$]?$` |
| `password_hash` | String | Required non-empty value accepted by `useradd`/`usermod`; not plaintext |
| `shell` | String | `bash`; names resolve under `/bin`, absolute paths are preserved |
| `groups` | String list | Empty; append supplementary memberships |
| `sudo` | Boolean | `false`; append `sudo` membership when true |

```yaml
users:
  - username: administrator
    password_hash: '${env:ADMIN_PASSWORD_HASH}'
    shell: bash
    groups: [adm]
    sudo: true
```

Generate a real hash with a tool such as `mkpasswd --method=SHA-512` and store it in `.env`. BuildOS checks only that the hash is non-empty. New users get a home directory; existing users have their hash and shell updated. Shells must be executable and groups must already exist. Relative shell paths containing `/` are rejected. Memberships are appended; `sudo: false` does not revoke existing access. The module does not delete users or groups.

## Paths and downloads

| Input | Resolution |
| --- | --- |
| CLI paths | Generator working directory |
| `base.include` | Containing YAML directory |
| `.env` | Main config directory |
| Local `source`, `assert`, `snap` | Target script working directory, unless absolute |
| File `destination` | Absolute target path |
| File `exec` | Destination's parent directory |
| `exec_cmd`, container `exec` | Target script working directory |

Launching a script does not change to its directory. Prefer absolute asset paths. Structured fields receive literal arguments after `${env:NAME}` substitution; `$ASSET_DIR/file` is not expanded. Shell evaluation applies only to explicit command fields.

HTTP/HTTPS assets are downloaded on the target with `wget --timeout=30 --tries=1` into distinct temporary files, removed on script exit. Other URI schemes are treated as local paths. Generation does not bundle or upload external assets.

## Execution constraints

- Scripts use `set -Eeuo pipefail`. Unhandled failures stop later operations and preserve the failing exit status. Completed changes remain; there is no rollback, checkpoint, or whole-script retry.
- Every run repeats all operations. Files and post-write commands run again; custom commands, snap installation, and named container creation may require explicit guards.
- Runtime files are written in place. New inline files start with restrictive permissions, but existing files retain their old permissions until the final `chmod`. Source-file copies also depend on source permissions and the runtime umask during writing. Do not rely on the requested final mode alone to protect secrets throughout deployment.
- Logs identify the source YAML, module occurrence, and failing generated line. Arguments are omitted from operation logs; invoked tools retain their stdout/stderr and may expose secrets.

Capture execution output with `sudo ./setup.sh >buildos.log 2>&1`. Success exits `0` and logs `Configuration completed`.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| Cannot generate output | Required flags, writable existing parent directory, output distinct from loaded inputs |
| Unknown field or invalid module | Reference tables, mapping/list shape, non-null values; use `exec_cmd` instead of `hooks` |
| Undefined variable | `.env` beside the main config; process environment is not consulted |
| Interpolated values omitted in an error | Inspect field names, types, and resolved values locally |
| Missing include or cycle | Relative include paths and active recursion chain |
| Root/distribution/release failure | Run as root on the configured target; place `base` first |
| Missing command, user/group, or shell | Prepare prerequisites before the dependent operation |
| Checksum failure | Digest of exact source bytes or downloaded response |
| systemd connection failure | Runtime service actions need systemd running |
| Unexpected action order | Main-file modules precede includes; package/service action order is fixed |
| Unexpected bytes or mode | YAML newline indicators, quoted modes, and later overwriting modules |
