# go-judge

[![Go Reference](https://pkg.go.dev/badge/github.com/criyle/go-judge.svg)](https://pkg.go.dev/github.com/criyle/go-judge) [![Go Report Card](https://goreportcard.com/badge/github.com/criyle/go-judge)](https://goreportcard.com/report/github.com/criyle/go-judge) [![Release](https://img.shields.io/github/v/tag/criyle/go-judge)](https://github.com/criyle/go-judge/releases/latest) ![Build](https://github.com/criyle/go-judge/workflows/Build/badge.svg)

[English](README.md)

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

```typescript
interface LocalFile {
    src: string; // 文件绝对路径
}

interface MemoryFile {
    content: string | Buffer; // 文件内容
}

interface PreparedFile {
    fileId: string; // 文件 id
}

interface Collector {
    name: string; // copyOut 文件名
    max: number;  // 最大大小限制
    pipe?: boolean; // 通过管道收集（默认值为false文件收集）
}

interface Symlink {
    symlink: string; // 符号连接目标 (v1.6.0+)
}

interface StreamIn {
    streamIn: boolean; // 流式输入 (v1.8.1+)
}

interface StreamOut {
    streamOut: boolean; // 流式输出 (v1.8.1+)
}

interface Cmd {
    args: string[]; // 程序命令行参数
    env?: string[]; // 程序环境变量

    // 指定 标准输入、标准输出和标准错误的文件 (null 是为了 pipe 的使用情况准备的，而且必须被 pipeMapping 的 in / out 指定)
    files?: (LocalFile | MemoryFile | PreparedFile | Collector | StreamIn | StreamOut | null)[];
    tty?: boolean; // 开启 TTY （需要保证标准输出和标准错误为同一文件）同时需要指定 TERM 环境变量 （例如 TERM=xterm）

    // 资源限制
    cpuLimit?: number;     // CPU时间限制，单位纳秒
    clockLimit?: number;   // 等待时间限制，单位纳秒 （通常为 cpuLimit 两倍）
    memoryLimit?: number;  // 内存限制，单位 byte
    stackLimit?: number;   // 栈内存限制，单位 byte
    procLimit?: number;    // 线程数量限制
    cpuRateLimit?: number; // 仅 Linux，CPU 使用率限制，1000 等于单核 100%
    cpuSetLimit?: string;  // 仅 Linux，限制 CPU 使用，使用方式和 cpuset cgroup 相同 （例如，`0` 表示限制仅使用第一个核）
    strictMemoryLimit?: boolean; // deprecated: 使用 dataSegmentLimit （这个选项依然有效）
    dataSegmentLimit?: boolean; // 仅linux，开启 rlimit 堆空间限制（如果不使用cgroup默认开启）
    addressSpaceLimit?: boolean; // 仅linux，开启 rlimit 虚拟内存空间限制（非常严格，在所以申请时触发限制）

    // 在执行程序之前复制进容器的文件列表
    copyIn?: {[dst:string]:LocalFile | MemoryFile | PreparedFile | Symlink};

    // 在执行程序后从容器文件系统中复制出来的文件列表
    // 在文件名之后加入 '?' 来使文件变为可选，可选文件不存在的情况不会触发 FileError
    copyOut?: string[];
    // 和 copyOut 相同，不过文件不返回内容，而是返回一个对应文件 ID ，内容可以通过 /file/:fileId 接口下载
    copyOutCached?: string[];
    // 指定 copyOut 复制文件大小限制，单位 byte
    copyOutMax?: number;
}

enum Status {
    Accepted = 'Accepted', // 正常情况
    MemoryLimitExceeded = 'Memory Limit Exceeded', // 内存超限
    TimeLimitExceeded = 'Time Limit Exceeded', // 时间超限
    OutputLimitExceeded = 'Output Limit Exceeded', // 输出超限
    FileError = 'File Error', // 文件错误
    NonzeroExitStatus = 'Nonzero Exit Status', // 非 0 退出值
    Signalled = 'Signalled', // 进程被信号终止
    InternalError = 'Internal Error', // 内部错误
}

interface PipeIndex {
    index: number; // cmd 的下标
    fd: number;    // cmd 的 fd
}

interface PipeMap {
    in: PipeIndex;  // 管道的输入端
    out: PipeIndex; // 管道的输出端
    // 开启管道代理，传输内容会从输出端复制到输入端
    // 输入端内容在输出端关闭以后会丢弃 （防止 SIGPIPE ）
    proxy?: boolean; 
    name?: string;   // 如果代理开启，内容会作为 copyOut 放在输入端 （用来 debug ）
    // 限制 copyOut 的最大大小，代理会在超出大小之后正常复制
    max?: number;    
}

enum FileErrorType {
    CopyInOpenFile = 'CopyInOpenFile',
    CopyInCreateFile = 'CopyInCreateFile',
    CopyInCopyContent = 'CopyInCopyContent',
    CopyOutOpen = 'CopyOutOpen',
    CopyOutNotRegularFile = 'CopyOutNotRegularFile',
    CopyOutSizeExceeded = 'CopyOutSizeExceeded',
    CopyOutCreateFile = 'CopyOutCreateFile',
    CopyOutCopyContent = 'CopyOutCopyContent',
    CollectSizeExceeded = 'CollectSizeExceeded',
}

interface FileError {
    name: string; // 错误文件名称
    type: FileErrorType; // 错误代码
    message?: string; // 错误信息
}

interface Request {
    requestId?: string; // 给 WebSocket 使用来区分返回值的来源请求
    cmd: Cmd[];
    pipeMapping: PipeMap[];
}

interface CancelRequest {
    cancelRequestId: string; // 取消某个正在进行中的请求
};

// WebSocket 请求
type WSRequest = Request | CancelRequest;

interface Result {
    status: Status;
    error?: string; // 详细错误信息
    exitStatus: number; // 程序返回值
    time: number;   // 程序运行 CPU 时间，单位纳秒
    memory: number; // 程序运行内存，单位 byte
    procPeak?: number; // 程序运行最大线程数量（需要内核版本>=6.1，且开启 cgroup v2）
    runTime: number; // 程序运行现实时间，单位纳秒
    // copyOut 和 pipeCollector 指定的文件内容
    files?: {[name:string]:string};
    // copyFileCached 指定的文件 id
    fileIds?: {[name:string]:string};
    // 文件错误详细信息
    fileError?: FileError[];
}

// WebSocket 结果
interface WSResult {
    requestId: string;
    results: Result[];
    error?: string;
}

// 流式请求 / 响应
interface Resize {
    index: number;
    fd: number;
    rows: number;
    cols: number;
    x: number;
    y: number;
}

interface Input {
    index: number;
    fd: number;
    content: Buffer;
}

interface Output {
    index: number;
    fd: number;
    content: Buffer;
}
```

### 示例

请使用 postman 或其他 REST API 调试工具向 http://localhost:5050/run 发送请求

<details><summary>单个c++文件编译运行</summary>

这个例子需要安装 `g++`。如果在 docker 环境中运行，请在容器中（`docker exec -it go-judge /bin/bash`）执行 `apt update && apt install g++`

需要注意，在生产环境中 `copyOutCached` 产生的文件在使用完之后需要使用（`DELETE /file/:id`）删除避免内存泄露

```json
{
    "cmd": [{
        "args": ["/usr/bin/g++", "a.cc", "-o", "a"],
        "env": ["PATH=/usr/bin:/bin"],
        "files": [{
            "content": ""
        }, {
            "name": "stdout",
            "max": 10240
        }, {
            "name": "stderr",
            "max": 10240
        }],
        "cpuLimit": 10000000000,
        "memoryLimit": 104857600,
        "procLimit": 50,
        "copyIn": {
            "a.cc": {
                "content": "#include <iostream>\nusing namespace std;\nint main() {\nint a, b;\ncin >> a >> b;\ncout << a + b << endl;\n}"
            }
        },
        "copyOut": ["stdout", "stderr"],
        "copyOutCached": ["a"]
    }]
}
```

```json
[
    {
        "status": "Accepted",
        "exitStatus": 0,
        "time": 303225231,
        "memory": 32243712,
        "runTime": 524177700,
        "files": {
            "stderr": "",
            "stdout": ""
        },
        "fileIds": {
            "a": "5LWIZAA45JHX4Y4Z"
        }
    }
]
```

```json
{
    "cmd": [{
        "args": ["a"],
        "env": ["PATH=/usr/bin:/bin"],
        "files": [{
            "content": "1 1"
        }, {
            "name": "stdout",
            "max": 10240
        }, {
            "name": "stderr",
            "max": 10240
        }],
        "cpuLimit": 10000000000,
        "memoryLimit": 104857600,
        "procLimit": 50,
        "copyIn": {
            "a": {
                "fileId": "5LWIZAA45JHX4Y4Z" // 这个缓存文件的 ID 来自上一个请求返回的 fileIds
            }
        }
    }]
}
```

```json
[
    {
        "status": "Accepted",
        "exitStatus": 0,
        "time": 1173000,
        "memory": 10637312,
        "runTime": 1100200,
        "files": {
            "stderr": "",
            "stdout": "2\n"
        }
    }
]
```

</details>

<details><summary>多个程序（例如交互题）</summary>

```json
{
    "cmd": [{
        "args": ["/bin/cat", "1"],
        "env": ["PATH=/usr/bin:/bin"],
        "files": [{
            "content": ""
        }, null, {
            "name": "stderr",
            "max": 10240
        }],
        "cpuLimit": 1000000000,
        "memoryLimit": 1048576,
        "procLimit": 50,
        "copyIn": {
            "1": { "content": "TEST 1" }
        },
        "copyOut": ["stderr"]
    },
    {
        "args": ["/bin/cat"],
        "env": ["PATH=/usr/bin:/bin"],
        "files": [null, {
            "name": "stdout",
            "max": 10240
        }, {
            "name": "stderr",
            "max": 10240
        }],
        "cpuLimit": 1000000000,
        "memoryLimit": 1048576,
        "procLimit": 50,
        "copyOut": ["stdout", "stderr"]
    }],
    "pipeMapping": [{
        "in" : {"index": 0, "fd": 1 },
        "out" : {"index": 1, "fd" : 0 }
    }]
}
```

```json
[
    {
        "status": "Accepted",
        "exitStatus": 0,
        "time": 1545123,
        "memory": 253952,
        "runTime": 4148800,
        "files": {
            "stderr": ""
        },
        "fileIds": {}
    },
    {
        "status": "Accepted",
        "exitStatus": 0,
        "time": 1501463,
        "memory": 253952,
        "runTime": 5897700,
        "files": {
            "stderr": "",
            "stdout": "TEST 1"
        },
        "fileIds": {}
    }
]
```

</details>

<details><summary>开启 CPURate 限制的死循环</summary>

```json
{
    "cmd": [{
    "args": ["/usr/bin/python3", "1.py"],
    "env": ["PATH=/usr/bin:/bin"],
    "files": [{"content": ""}, {"name": "stdout","max": 10240}, {"name": "stderr","max": 10240}],
    "cpuLimit": 3000000000,
    "clockLimit": 4000000000,
    "memoryLimit": 104857600,
    "procLimit": 50,
    "cpuRate": 0.1,
    "copyIn": {
        "1.py": {
            "content": "while True:\n    pass"
        }
    }}]
}
```

```json
[
    {
        "status": "Time Limit Exceeded",
        "exitStatus": 9,
        "time": 414803599,
        "memory": 3657728,
        "runTime": 4046054900,
        "files": {
            "stderr": "",
            "stdout": ""
        }
    }
]
```

</details>

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

#### 编译 docker

终端中运行 `docker build -t go-judge -f Dockerfile.exec .`

沙箱服务需要特权级别 docker 来创建子容器和提供 cgroup 资源限制。

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

#### cgroup v2

`go-judge` 目前已经支持 cgroup v2 鉴于越来越多的 Linux 发行版默认启用 cgroup v2 而不是 v1 （比如 Ubuntu 21.10+，Fedora 31+）。然而，对于内核版本小于 5.19 的版本，因为 cgroup v2 在内存控制器里面缺少 `memory.max_usage_in_bytes`，内存使用量计数会转而采用 `maxrss` 指标。这项指标会显示的比使用 cgroup v1 时候要稍多，在运行使用内存较少的程序时比较明显。对于内核版本大于或等于 5.19 的版本，`memory.peak` 会被采用。

同时，如果本程序在容器中运行，容器中的进程会被移到 `/api` cgroup v2 控制器中来开启 cgroup v2 嵌套支持。

在 `systemd` 为 `init` 的发行版中运行时，`go-judge` 会使用 `dbus` 通知 `systemd` 来创建一个临时 `scope` 作为 `cgroup` 的根。

在高于 5.7 的内核中运行时，`go-judge` 会尝试更快的 `clone3(CLONE_INTO_CGROUP)` 方法.

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

### 压力测试

使用 `wrk` 和 `t.lua`: `wrk -s t.lua -c 1 -t 1 -d 30s --latency http://localhost:5050/run`.

注意，这些结果只是极限情况下的表现，实际情况和使用方式相关。通常沙箱服务相比于直接运行程序，通常有 1 毫秒左右额外延迟。

```lua
wrk.method = "POST"
wrk.body   = '{"cmd":[{"args":["/bin/cat","a.hs"],"env":["PATH=/usr/bin:/bin"],"files":[{"content":""},{"name":"stdout","max":10240},{"name":"stderr","max":10240}],"cpuLimit":10000000000,"memoryLimit":104857600,"procLimit":50,"copyIn":{"a.hs":{"content":"main = putStrLn \\"Hello, World!\\""},"b":{"content":"TEST"}}}]}'
wrk.headers["Content-Type"] = "application/json;charset=UTF-8"
```

- 单线程 ~800-860 op/s Windows 10 WSL2 @ 5800X
- 多线程 ~4500-6000 op/s Windows 10 WSL2 @ 5800X

单线程:

```text
Running 30s test @ http://localhost:5050/run
  1 threads and 1 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency     1.16ms  132.89us   6.20ms   90.15%
    Req/Sec     0.87k    19.33     0.91k    85.33%
  Latency Distribution
     50%    1.13ms
     75%    1.18ms
     90%    1.27ms
     99%    1.61ms
  25956 requests in 30.01s, 6.88MB read
Requests/sec:    864.88
Transfer/sec:    234.68KB
```
