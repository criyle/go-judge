# go-judge

[![Go Reference](https://pkg.go.dev/badge/github.com/criyle/go-judge.svg)](https://pkg.go.dev/github.com/criyle/go-judge) [![Go Report Card](https://goreportcard.com/badge/github.com/criyle/go-judge)](https://goreportcard.com/report/github.com/criyle/go-judge) [![Release](https://img.shields.io/github/v/tag/criyle/go-judge)](https://github.com/criyle/go-judge/releases/latest) ![Build](https://github.com/criyle/go-judge/workflows/Build/badge.svg)

[中文文档](README.cn.md) | [Documentation](https://docs.goj.ac)

Fast, Simple, Secure

## Quick Start

### Install & Run

Download compiled executable `go-judge` for your platform from [Release](https://github.com/criyle/go-judge/releases) and run.

Or, by docker

```bash
docker run -it --rm --privileged --shm-size=256m -p 5050:5050 --name=go-judge criyle/go-judge
```

### REST API

A REST service to run program in restricted environment (Listening on `localhost:5050` by default).

- **POST /run execute program in the restricted environment**
- GET /file list all cached file id to original name map
  - POST /file prepare a file in the go judge (in memory), returns fileId (can be referenced in /run parameter)
  - GET /file/:fileId downloads file from go judge (in memory), returns file content
  - DELETE /file/:fileId  delete file specified by fileId
- /ws WebSocket version for /run
- /stream WebSocket for stream run. Supports streaming interface
- GET /version gets build git version (e.g. `v1.9.0`) together with runtime information (go version, os, platform)
  - GET /config gets some configuration (e.g. `fileStorePath`, `runnerConfig`) together with some supported features

### REST API Interface

[API Interface Structure Definition](https://docs.goj.ac/api#rest-api-interface)

### Example Request & Response

[Example Request & Response](https://docs.goj.ac/example)

## Documentation

### Prerequisite

- Linux Kernel Version >= 3.10
- Cgroup file system mounted at /sys/fs/cgroup. Usually done by systemd

### Architecture

```text
+----------------------------------------------------------------------------------+
| Transport Layer (HTTP / WebSocket / FFI / ...)                                   |
+----------------------------------------------------------------------------------+
| Sandbox Worker (Environment Pool w/ Environment Builder )                        |
+-----------------------------------------------------------+----------------------+
| EnvExec                                                   | File Store           |
+--------------------+----------------+---------------------+---------------+------+
| Linux (go-sandbox) | Windows (winc) | macOS (app sandbox) | Shared Memory | Disk |
+--------------------+----------------+---------------------+---------------+------+
```

### Configurations

Server:

- The default binding address for the go judge is `localhost:5050`. Can be specified with `-http-addr` flag.
- By default gRPC endpoint is disabled, to enable gRPC endpoint, add `-enable-grpc` flag.
  - The default binding address for the gRPC go judge is `localhost:5051`. Can be specified with `-grpc-addr` flag.
- The default log level is info, use `-silent` to disable logs or use `-release` to enable release logger (auto turn on if in docker).
- `-auth-token` to add token-based authentication to REST / gRPC
- By default, the GO debug endpoints (`localhost:5052/debug`) are disabled, to enable, specifies `-enable-debug`, and it also enables debug log
- By default, the prometheus metrics endpoints (`localhost:5052/metrics`) are disabled, to enable, specifies `-enable-metrics`
- Monitoring HTTP endpoint is enabled if metrics / debug is enabled, the default addr is `localhost:5052` and can be specified by `-monitor-addr`

Sandbox:

- The default concurrency equal to number of CPU, Can be specified with `-parallelism` flag.
- `-mount-conf` specifies detailed mount configuration, please refer [File System Mount](https://docs.goj.ac/mount) as a reference (Linux only)
- `-file-timeout` specifies maximum TTL for file created in file store （e.g. `30m`)
- The default file store is in memory(`/dev/shm/`), local cache can be specified with `-dir` flag.
- `-output-limit` specifies size limit of POSIX rlimit of output (default 256MiB)
- `-copy-out-limit` specifies the default file copy out max (default 64MiB)

You can find [more available configuration here](https://docs.goj.ac/configuration).

### Run Terminal in the Container

Download `go-judge-shell` from [Release](https://github.com/criyle/go-judge/releases) and run. It will connect local `go-judge`, and open an interactive shell in the container for debugging purpose.

### Return Status

- Accepted: Program exited with status code 0 within time & memory limits
- Memory Limit Exceeded: Program uses more memory than memory limits
- Time Limit Exceeded: (`exitStatus` usually have value `9` as killed by `SIGKILL` after timeout)
  - Program uses more CPU time than cpuLimit
  - Or, program uses more clock time than clockLimit
- Output Limit Exceeded:
  - Program output more than pipeCollector limits
  - Or, program output more than output-limit
- File Error:
  - CopyIn file is not existed
  - Or, CopyIn file too large for container file system
  - Or, CopyOut file is not existed after program exited
- Non Zero Exit Status: Program exited with non 0 status code within time & memory limits
- Signalled: Program exited with signal (e.g. `SIGSEGV`)
- Dangerous Syscall: Program killed by seccomp filter (not enabled by default)
- Internal Error:
  - Program is not exist
  - Or, container create not successful (e.g. not privileged docker)
  - Or, other errors

### Container Root Filesystem

For linux platform, the default mounts points are bind mounting host's `/lib`, `/lib64`, `/usr`, `/bin`, `/etc/ld.so.cache`, `/etc/alternatives`, `/etc/fpc.cfg`, `/dev/null`, `/dev/urandom`, `/dev/random`, `/dev/zero`, `/dev/full` and mounts tmpfs at `/w`, `/tmp` and creates `/proc`.

To [customize mount points](https://docs.goj.ac/mount#customization), please look at example `mount.yaml` file.

If `mount.yaml` is not specified, the size of `tmpfs` for `/w` and `/tmp` is configured through `-tmp-fs-param` with default value `size=128m,nr_inodes=4k`

If a file named `/.env` exists in the container rootfs, the container will load the file as environment variable line by line.

If a bind mount is specifying a target within the previous mounted one, please ensure the target exists in the previous mount point.

### Metrics Monitoring Endpoint

[Prometheus Metrics Monitoring Endpoint](https://docs.goj.ac/api#prometheus-monitor-api)

### Notice

> [!WARNING]  
> Window and macOS support are experimental and should not be used in production environments

#### cgroup usage

For cgroup v1, the `go-judge` need root privilege to create `cgroup`. Either creates sub-directory `/sys/fs/cgroup/cpuacct/gojudge`, `/sys/fs/cgroup/memory/gojudge`, `/sys/fs/cgroup/pids/gojudge` and make execution user readable or use `sudo` to run it.

For cgroup v2, systemd dbus will be used to create a transient scope for cgroup integration.

If no permission to create cgroup, the cgroup related limit will not be effective.

#### cgroup v2 support

The cgroup v2 is supported by `go-judge` now when running as root since more Linux distribution are enabling cgroup v2 by default (e.g. Ubuntu 21.10+, Fedora 31+). However, for kernel < 5.19, due to missing `memory.max_usage_in_bytes` in `memory` controller, the memory usage is now accounted by `maxrss` returned by `wait4` syscall. Thus, the memory usage appears higher than those who uses cgroup v1. For kernel >= 5.19, `memory.peak` is being used.

When running in containers, the `go-judge` will migrate all processed into `/api` hierarchy to enable nesting support.

When running in Linux distributions powered by `systemd`, the `go-judge` will contact `systemd` via `dbus` to create a transient scope as cgroup root.

When running with kernel >= 5.7, the `go-judge` will try faster `clone3(CLONE_INTO_CGROUP)` and `vfork` method.

#### Memory Usage

The controller usually consumes `20M` memory and each container usually consumes `20M` + size of tmpfs `2 * 128M`. For each request, it consumes as much as user program limit + extra limit (`16k`) + total copy out max. Notice that the cached file stores in the shared memory (`/dev/shm`) of the host, so please ensure enough size allocated.

For example, when concurrency = 4, the container itself can consume as much as `60 + (20+32) * 4M = 268M` + 4 * total copy out + total max memory of requests.

Due to limitation of GO runtime, the memory will not return to OS automatically, which could lead to OOM killer. A background worker was introduced to checks heap usage and invokes GC when necessary.

- `-force-gc-target` default `20m`, the minimal size to trigger GC
- `-force-gc-interval` default `5s`, the interval to check memory usage
