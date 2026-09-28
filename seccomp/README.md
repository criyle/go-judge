# Seccomp profiles

## Moby source and license

`moby-default.json` is a vendored snapshot derived from Moby's default
seccomp profile:

- Source: <https://github.com/moby/profiles/blob/main/seccomp/default.json>
- License: Apache License 2.0, as specified by the Moby Profiles repository:
  <https://github.com/moby/profiles/blob/main/LICENSE>

The generated `moby.yaml` contains go-judge-specific architecture processing
and deliberately removes selected unused, high-risk syscall groups; see
`cmd/seccomp-convert/main.go` for those changes.

`moby.yaml` is generated from `moby-default.json`:

```sh
go run ./cmd/seccomp-convert \
  -input seccomp/moby-default.json \
  -output seccomp/moby.yaml
```

The YAML schema has a top-level `default_action`, a shared `base` policy, and
per-architecture additions:

```yaml
default_action: errno
base:
  syscalls: [...]
architectures:
  x86_64:
    syscalls: [...]
  aarch64:
    syscalls: [...]
```

The converter puts only syscall groups common to every supported architecture
in `base`, and resolves Moby architecture selectors into each architecture's
additions. Runtime loading merges `base` with the selected additions and sends
the result directly to Elastic's assembler; it does not modify or filter
syscall groups.

The default profile is embedded into the go-judge binary. Pass `-seccomp-conf
<path>` to use an external profile instead.

The generated policy is intentionally stricter than the Moby baseline. In
addition to `personality`, it denies unused groups for cross-process
inspection/control, fileless executable loading, legacy x86 execution state,
device-node creation, filesystem event attachment, cross-process
signaling/scheduling, legacy asynchronous I/O, alternate filesystem/mount
interfaces, and kernel-mediated data transfer. The source list in
`cmd/seccomp-convert/main.go` documents the rationale for each group.
The container init installs the generated policy after namespace and
filesystem initialization. Workload processes inherit that filter; they do not
install a second filter for each command. `clone`, `clone3`, and `unshare` are
allowed because the container init and cgroup placement paths require them.
