# go-judge

[![Go Reference](https://pkg.go.dev/badge/github.com/criyle/go-judge.svg)](https://pkg.go.dev/github.com/criyle/go-judge) [![Go Report Card](https://goreportcard.com/badge/github.com/criyle/go-judge)](https://goreportcard.com/report/github.com/criyle/go-judge) [![Release](https://img.shields.io/github/v/tag/criyle/go-judge)](https://github.com/criyle/go-judge/releases/latest) ![Build](https://github.com/criyle/go-judge/workflows/Build/badge.svg)

[English](README.md) | [文档](https://docs.goj.ac/cn)

快速，简单，安全

## 快速上手

### 安装和运行

下载对应平台预编译二进制文件 `go-judge` [Release](https://github.com/criyle/go-judge/releases) 并在终端开启

或者使用 docker

```bash
docker run -it --rm --privileged --shm-size=256m -p 5050:5050 --name=go-judge criyle/go-judge
```

### REST API 接口

沙箱服务提供 REST API 接口来在受限制的环境中运行程序（默认监听于 `localhost:5050`）。

- **POST /run 在受限制的环境中运行程序**
- GET /file 得到所有在文件存储中的文件 ID 到原始命名映射
  - POST /file 上传一个文件到文件存储，返回一个文件 ID 用于提供给 /run 接口
  - GET /file/:fileId 下载文件 ID 指定的文件
  - DELETE /file/:fileId 删除文件 ID 指定的文件
- /ws /run 接口的 WebSocket 版
- /stream 运行交互式命令。支持流式 api
- /version 获取构建的 Git 版本 (例如 v1.9.0) 以及运行时信息 (go 版本, 操作系统, 平台)
  - /config 获取部分配置信息 (例如 fileStorePath, runnerConfig) 以及支持的功能特性

### REST API 接口定义

[接口数据类型定义](https://docs.goj.ac/cn/api#rest-api-接口定义)

### 示例

请使用 postman 或其他 REST API 调试工具向 http://localhost:5050/run 发送请求

[请求实例](https://docs.goj.ac/cn/example)

## 进阶设置

### 运行要求

- Linux 内核版本 >= 3.10
- 系统 Cgroup 文件系统挂载于 `/sys/fs/cgroup`（Systemd 默认）

### 系统架构

```text
+----------------------------------------------------------------------------+
| 传输层 (HTTP / WebSocket / FFI / ...)                                      |
+----------------------------------------------------------------------------+
| 工作协程 (运行环境池 和 运行环境生产者 )                                     |
+-----------------------------------------------------------+----------------+
| 运行环境                                                   | 文件存储       |
+--------------------+----------------+---------------------+----------+-----+
| Linux (go-sandbox) | Windows (winc) | macOS (app sandbox) | 共享内存 | 磁盘 |
+--------------------+----------------+---------------------+----------+-----+
```

### 配置

服务相关:

- 默认监听地址是 `localhost:5050`，使用 `-http-addr` 指定
- 默认 gRPC 接口处于关闭状态，使用 `-enable-grpc` 开启
  - 默认 gRPC 监听地址是 `localhost:5051` ，使用 `-grpc-addr` 指定
- 默认日志等级是 info ，使用 `-silent` 关闭 或 使用 `-release` 开启 release 级别日志(在 docker 中会自动开启)
- 默认没有开启鉴权，使用 `-auth-token` 指定令牌鉴权
- 默认没有开启 go 语言调试接口（`localhost:5052/debug`），使用 `-enable-debug` 开启，同时将日志层级设为 Debug
- 默认没有开启 prometheus 监控接口，使用 `-enable-metrics` 开启 `localhost:5052/metrics`
- 在启用 go 语言调试接口或者 prometheus 监控接口的情况下，默认监控接口为 `localhost:5052`，使用 `-monitor-addr` 指定

沙箱相关:

- 默认同时运行任务数为和 CPU 数量相同，使用 `-parallelism` 指定
- 使用 `-mount-conf` 指定沙箱文件系统挂载细节，详细请参见 [文件系统挂载](https://docs.goj.ac/cn/mount) (仅 Linux)
- 使用 `-file-timeout` 指定文件存储文件最大时间。超出时间的文件将会删除。（例如指定 `30m` 时，缓存文件将在创建后 30 分钟删除）
- 默认文件存储在共享内存文件系统中（`/dev/shm/`），可以使用 `-dir` 指定另外的本地目录为文件存储
- 默认最大输出限制为 `256MiB`，使用 `-output-limit` 指定 POSIX rlimit 的输出限制
- 默认最大 `copyOut` 文件大小为 `64MiB` ，使用 `-copy-out-limit` 指定

可以[在此查看更多配置文档](https://docs.goj.ac/cn/configuration)。

### 指标监控

[Prometheus 指标监控接口](https://docs.goj.ac/cn/api#prometheus-监控接口)

### 在沙箱中运行终端

从 [Release](https://github.com/criyle/go-judge/releases) 下载 `go-judge-shell` 。运行将连接本地 `go-judge` 沙箱服务并开启一个容器内的终端用于调试。

### /run 接口返回状态

- Accepted: 程序在资源限制内正常退出
- Memory Limit Exceeded: 超出内存限制
- Time Limit Exceeded: （通常 `exitStatus` 为 `9`（超时时被 `SIGKILL` 结束））
  - 超出 `timeLimit` 时间限制
  - 或者超过 `clockLimit` 等待时间限制
- Output Limit Exceeded:
  - 超出 `pipeCollector` 限制
  - 或者超出 `-output-limit` 最大输出限制
- File Error:
  - `copyIn` 指定文件不存在
  - 或者 `copyIn` 指定文件大小超出沙箱文件系统限制
  - 或者 `copyOut` 指定文件不存在
- Non Zero Exit Status: 程序用非 0 返回值退出
- Signalled: 程序收到结束信号而退出（例如 `SIGSEGV`）
- Dangerous Syscall: 程序被 `seccomp` 过滤器结束（默认不启用）
- Internal Error:
  - 指定程序路径不存在
  - 或者容器创建失败（比如使用非特权 docker）
  - 或者其他错误

### 容器的文件系统

在 Linux 平台，默认只读挂载点包括主机的 `/lib`, `/lib64`, `/usr`, `/bin`, `/etc/ld.so.cache`, `/etc/alternatives`, `/etc/fpc.cfg`, `/dev/null`, `/dev/urandom`, `/dev/random`, `/dev/zero`, `/dev/full` 和临时文件系统 `/w`, `/tmp` 以及 `/proc`。

使用 `mount.yaml` [定制容器文件系统](https://docs.goj.ac/cn/mount#%E8%87%AA%E5%AE%9A%E4%B9%89%E6%8C%82%E8%BD%BD)。

不使用 `mount.yaml` 时，`/w` 的 `/tmp` 挂载 `tmpfs` 大小通过 `-tmp-fs-param` 指定，默认值为 `size=128m,nr_inodes=4k`

如果在容器的根目录存在 `/.env` 文件，那么这个文件会在容器创建时被载入。文件的每一行会作为环境变量的初始值加入到运行程序当中。

如果之后指定的挂载点目标在之前的挂载点之下，那么需要保证之前的挂载点存在目标文件或者文件夹。

### 注意

> [!WARNING]  
> Windows 和 macOS 平台为实验性支持，请不要在生产环境使用

#### 使用 cgroup

在 cgroup v1 系统上 `go-judge` 需要 `root` 权限创建 `cgroup`。请使用 `sudo` 以 `root` 用户运行或者确保运行用户拥有以下目录的读写权限 `/sys/fs/cgroup/cpuacct/gojudge`, `/sys/fs/cgroup/memory/gojudge`, `/sys/fs/cgroup/pids/gojudge`。

在 cgroup v2 系统上，`go-judge` 会和 `system dbus` 沟通，创建一个临时 `scope`。如果 `systemd` 不存在，并且拥有 `root` 权限那么将尝试进行嵌套初始化。

如果没有 `cgroup` 的权限，那么 `cgroup` 相关的资源配置将不会生效。

#### cgroup v2

`go-judge` 目前已经支持 cgroup v2 鉴于越来越多的 Linux 发行版默认启用 cgroup v2 而不是 v1 （比如 Ubuntu 21.10+，Fedora 31+）。然而，对于内核版本小于 5.19 的版本，因为 cgroup v2 在内存控制器里面缺少 `memory.max_usage_in_bytes`，内存使用量计数会转而采用 `maxrss` 指标。这项指标会显示的比使用 cgroup v1 时候要稍多，在运行使用内存较少的程序时比较明显。对于内核版本大于或等于 5.19 的版本，`memory.peak` 会被采用。

同时，如果本程序在容器中运行，容器中的进程会被移到 /api cgroup v2 层级中来开启 cgroup v2 嵌套支持。

在 `systemd` 为 `init` 的发行版中运行时，`go-judge` 会使用 `dbus` 通知 `systemd` 来创建一个临时 `scope` 作为 `cgroup` 的根。

在高于 5.7 的内核中运行时，`go-judge` 会尝试更快的 `clone3(CLONE_INTO_CGROUP)` 和 `vfork` 方法.

#### 内存使用

控制进程通常会使用 `20M` 内存。每个容器进程通常会占用 `20M` 内存 + 临时文件系统 (tmpfs) 大小 `2 * 128M`。对于每个请求，它将占用 用户程序限制的最大内存 + 额外限制 (`16k`) + 总 copy out 最大限制。请注意，缓存文件存储在宿主机的共享内存中 (`/dev/shm`)，因此请确保分配了足够的空间。

比方说当并发数（concurrency）为 4 时，容器本身可能占用高达 `60 + (20+32) * 4M = 268M` + 4 \* 总 copy out 限制 + 总请求的最大内存限制。

由于 Go 语言运行时（runtime）的限制，内存并不会自动返回给操作系统，这可能会导致 OOM Killer 杀死进程。因此引入了一个后台工作线程，用于检查堆内存使用情况并在必要时调用垃圾收集（GC）。

- `-force-gc-target` 默认 `20m`，触发 GC 的最小堆内存使用量
- `-force-gc-interval` 默认 `5s`，检查内存使用情况的间隔时间
