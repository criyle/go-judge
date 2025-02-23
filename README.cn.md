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

- **/run POST 在受限制的环境中运行程序（下面有例子）**
- /file GET 得到所有在文件存储中的文件 ID 到原始命名映射
- /file POST 上传一个文件到文件存储，返回一个文件 ID 用于提供给 /run 接口
- /file/:fileId GET 下载文件 ID 指定的文件
- /file/:fileId DELETE 删除文件 ID 指定的文件
- /ws /run 接口的 WebSocket 版
- /stream 运行交互式命令
- /version 得到本程序编译版本和 go 语言运行时版本
- /config 得到本程序部分运行参数，包括沙箱详细参数

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

### 命令行参数

服务相关:

- 默认监听地址是 `localhost:5050`，使用 `-http-addr` 指定
- 默认 gRPC 接口处于关闭状态，使用 `-enable-grpc` 开启
- 默认 gRPC 监听地址是 `localhost:5051` ，使用 `-grpc-addr` 指定
- 默认日志等级是 info ，使用 `-silent` 关闭 或 使用 `-release` 开启 release 级别日志
- 默认没有开启鉴权，使用 `-auth-token` 指定令牌鉴权
- 默认没有开启 go 语言调试接口（`localhost:5052/debug`），使用 `-enable-debug` 开启，同时将日志层级设为 Debug
- 默认没有开启 prometheus 监控接口，使用 `-enable-metrics` 开启 `localhost:5052/metrics`
- 在启用 go 语言调试接口或者 prometheus 监控接口的情况下，默认监控接口为 `localhost:5052`，使用 `-monitor-addr` 指定

沙箱相关:

- 默认同时运行任务数为和 CPU 数量相同，使用 `-parallelism` 指定
- 默认文件存储在内存里，使用 `-dir` 指定本地目录为文件存储
- 默认 cgroup 的前缀为 `gojudge` ，使用 `-cgroup-prefix` 指定
- 默认没有磁盘文件复制限制，使用 `-src-prefix` 限制 copyIn 操作文件目录前缀，使用逗号 `,` 分隔（需要绝对路径）（例如：`/bin,/usr`）
- 默认时间和内存使用检查周期为 100 毫秒(`100ms`)，使用 `-time-limit-checker-interval` 指定
- 默认最大输出限制为 `256MiB`，使用 `-output-limit` 指定
- 默认最大打开文件描述符为 `256`，使用 `-open-file-limit` 指定
- 默认最大额外内存使用为 `16KiB` ，使用 `-extra-memory-limit` 指定
- 默认最大 `copyOut` 文件大小为 `64MiB` ，使用 `-copy-out-limit` 指定
- 使用 `-cpuset` 指定 `cpuset.cpus` （仅 Linux）
- 默认容器用户开始区间为 0（不启用） 使用 `-container-cred-start` 指定（仅 Linux）
  - 举例，默认情况下第 0 个容器使用 10001 作为容器用户。第 1 个容器使用 10002 作为容器用户，以此类推
- 使用 `-enable-cpu-rate` 开启 `cpu` cgroup 来启用 `cpuRate` 控制（仅 Linux）
  - 使用 `-cpu-cfs-period` 指定 cfs_period if cpu rate is enabled (default 100ms) (valid value: \[1ms, 1s\])
- 使用 `-seccomp-conf` 指定 `seecomp` 过滤器（需要编译标志 `seccomp`，默认不开启）（仅 Linux）
- 使用 `-pre-fork` 指定启动时创建的容器数量
- 使用 `-tmp-fs-param` 指定容器内 `tmpfs` 的挂载参数（仅 Linux）
- 使用 `-file-timeout` 指定文件存储文件最大时间。超出时间的文件将会删除。（举例 `30m`）
- 使用 `-mount-conf` 指定沙箱文件系统挂载细节，详细请参见 `mount.yaml` (仅 Linux)
- 使用 `-container-init-path` 指定 `cinit` 路径 (请不要使用，仅 debug) (仅 Linux)

### 环境变量

所有命令行参数都可以通过环境变量的形式来指定，（类似 `ES_HTTP_ADDR` 来指定 `-http-addr`）。使用 `go-judge --help` 查看所有环境变量

### 编译沙箱终端

编译 `go build ./cmd/go-judge-shell`

运行 `./go-judge-shell`，需要打开 gRPC 接口来使用。提供一个沙箱内的终端环境。

### /run 接口返回状态

- Accepted: 程序在资源限制内正常退出
- Memory Limit Exceeded: 超出内存限制
- Time Limit Exceeded:
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
- Dangerous Syscall: 程序被 `seccomp` 过滤器结束
- Internal Error:
  - 指定程序路径不存在
  - 或者容器创建失败
    - 比如使用非特权 docker
    - 或者在个人目录下以 root 权限运行
  - 或者其他错误

### 容器的文件系统

在 Linux 平台，默认只读挂载点包括主机的 `/lib`, `/lib64`, `/usr`, `/bin`, `/etc/ld.so.cache`, `/etc/alternatives`, `/etc/fpc.cfg`, `/dev/null`, `/dev/urandom`, `/dev/random`, `/dev/zero`, `/dev/full` 和临时文件系统 `/w`, `/tmp` 以及 `/proc`。

使用 `mount.yaml` 定制容器文件系统。

`/w` 的 `/tmp` 挂载 `tmpfs` 大小通过 `-tmp-fs-param` 指定，默认值为 `size=128m,nr_inodes=4k`

如果在容器的根目录存在 `/.env` 文件，那么这个文件会在容器创建时被载入。文件的每一行会作为环境变量的初始值加入到运行程序当中。

如果之后指定的挂载点目标在之前的挂载点之下，那么需要保证之前的挂载点存在目标文件或者文件夹。

### 包

- envexec: 核心逻辑包，在提供的环境中运行一个或多个程序
- env: 环境的标准实现

### 注意

#### 使用 cgroup

在 cgroup v1 系统上 `go-judge` 需要 `root` 权限创建 `cgroup`。请使用 `sudo` 以 `root` 用户运行或者确保运行用户拥有以下目录的读写权限 `/sys/fs/cgroup/cpuacct/gojudge`, `/sys/fs/cgroup/memory/gojudge`, `/sys/fs/cgroup/pids/gojudge`。

在 cgroup v2 系统上，`go-judge` 会和 `system dbus` 沟通，创建一个临时 `scope`。如果 `systemd` 不存在，并且拥有 `root` 权限那么将尝试进行嵌套初始化。

如果没有 `cgroup` 的权限，那么 `cgroup` 相关的资源配置将不会生效。

#### cgroup v2

`go-judge` 目前已经支持 cgroup v2 鉴于越来越多的 Linux 发行版默认启用 cgroup v2 而不是 v1 （比如 Ubuntu 21.10+，Fedora 31+）。然而，对于内核版本小于 5.19 的版本，因为 cgroup v2 在内存控制器里面缺少 `memory.max_usage_in_bytes`，内存使用量计数会转而采用 `maxrss` 指标。这项指标会显示的比使用 cgroup v1 时候要稍多，在运行使用内存较少的程序时比较明显。对于内核版本大于或等于 5.19 的版本，`memory.peak` 会被采用。

同时，如果本程序在容器中运行，容器中的进程会被移到 `/api` cgroup v2 控制器中来开启 cgroup v2 嵌套支持。

在 `systemd` 为 `init` 的发行版中运行时，`go-judge` 会使用 `dbus` 通知 `systemd` 来创建一个临时 `scope` 作为 `cgroup` 的根。

在高于 5.7 的内核中运行时，`go-judge` 会尝试更快的 `clone3(CLONE_INTO_CGROUP)` 和 `vfork` 方法.

#### 内存使用

控制进程通常会使用 `20M` 内存，每个容器进程最大会使用 `20M` 内存，每个请求最大会使用 `2 * 16M` + 总 copy out max 限制 * 2 内存。请注意，缓存文件会存储在宿主机的共享内存中 (`/dev/shm`)，请保证其大小足够存储运行时最大可能文件。

比方说当同时请求数最大为 4 的时候，本程序最大会占用 `60 + (20+32) * 4M = 268M` + 总 copy out max 限制 * 8 内存 + 总运行程序最大内存限制。

因为 go 语言 runtime 垃圾收集算法实现的问题，它并不会主动归还占用内存。这种情况可能会引发 OOM Killer 杀死进程。加入了一个后台检查线程用于在堆内存占用高时强制垃圾收集和归还内存。

- `-force-gc-target` 默认 `20m`, 堆内存使用超过该值是强制垃圾收集和归还内存
- `-force-gc-interval` 默认 `5s`, 为后台线程检查的频繁程度

### WebSocket 流接口

WebSocket 流接口是用于运行一个程序，同时和它的输入输出进行交互。所有的消息都应该使用 WebSocket 的 binary 格式来发送来避免兼容性问题。

```text
+--------+--------+---...
| 类型   | 载荷 ...
+--------|--------+---...
请求:
请求类型 = 
  1 - 运行请求 (载荷 = JSON 编码的请求体)
  2 - 设置终端窗口大小 (载荷 = JSON 编码的请求体)
  3 - 输入 (载荷 = 1 字节 (4 位的 命令下标 + 4 位的 文件描述符) + 输入内容)
  4 - 取消 (没有载荷)

响应:
响应类型 = 
  1 - 运行结果 (载荷 = JSON 编码的运行结果)
  2 - 输出 (载荷 = 1 字节 (4 位的 命令下标 + 4 位的 文件描述符) + 输入内容)
```

任何的不完整，或者不合法的消息会被认为是错误，并终止运行。
