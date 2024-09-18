# go-judge

[![Go Reference](https://pkg.go.dev/badge/github.com/criyle/go-judge.svg)](https://pkg.go.dev/github.com/criyle/go-judge) [![Go Report Card](https://goreportcard.com/badge/github.com/criyle/go-judge)](https://goreportcard.com/report/github.com/criyle/go-judge) [![Release](https://img.shields.io/github/v/tag/criyle/go-judge)](https://github.com/criyle/go-judge/releases/latest) ![Build](https://github.com/criyle/go-judge/workflows/Build/badge.svg)

[中文文档](README.cn.md)

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

- **/run POST execute program in the restricted environment (examples below)**
- /file GET list all cached file id to original name map
- /file POST prepare a file in the go judge (in memory), returns fileId (can be referenced in /run parameter)
- /file/:fileId GET downloads file from go judge (in memory), returns file content
- /file/:fileId DELETE delete file specified by fileId
- /ws WebSocket for /run
- /stream WebSocket for stream run
- /version gets build git version (e.g. `v1.4.0`) together with runtime information (go version, os, platform)
- /config gets some configuration (e.g. `fileStorePath`, `runnerConfig`) together with some supported features

### REST API Interface

```typescript
interface LocalFile {
    src: string; // absolute path for the file
}

interface MemoryFile {
    content: string | Buffer; // file contents
}

interface PreparedFile {
    fileId: string; // fileId defines file uploaded by /file
}

interface Collector {
    name: string; // file name in copyOut
    max: number;  // maximum bytes to collect from pipe
    pipe?: boolean; // collect over pipe or not (default false)
}

interface Symlink {
    symlink: string; // symlink destination (v1.6.0+)
}

interface StreamIn {
    streamIn: boolean; // stream input (v1.8.1+)
}

interface StreamOut {
    streamOut: boolean; // stream output (v1.8.1+)
}

interface Cmd {
    args: string[]; // command line argument
    env?: string[]; // environment

    // specifies file input / pipe collector for program file descriptors (null is reserved for pipe mapping and must be filled by in / out)
    files?: (LocalFile | MemoryFile | PreparedFile | Collector | StreamIn | StreamOut ｜ null)[];
    tty?: boolean; // enables tty on the input and output pipes (should have just one input & one output)
    // Notice: must have TERM environment variables (e.g. TERM=xterm)

    // limitations
    cpuLimit?: number;     // ns
    realCpuLimit?: number; // deprecated: use clock limit instead (still working)
    clockLimit?: number;   // ns
    memoryLimit?: number;  // byte
    stackLimit?: number;   // byte (N/A on windows, macOS cannot set over 32M)
    procLimit?: number;
    cpuRateLimit?: number; // limit cpu usage (1000 equals 1 cpu)
    cpuSetLimit?: string; // Linux only: set the cpuSet for cgroup
    strictMemoryLimit?: boolean; // deprecated: use dataSegmentLimit instead (still working)
    dataSegmentLimit?: boolean; // Linux only: use (+ rlimit_data limit) enable by default if cgroup not enabled
    addressSpaceLimit?: boolean; // Linux only: use (+ rlimit_address_space limit) 

    // copy the correspond file to the container dst path
    copyIn?: {[dst:string]:LocalFile | MemoryFile | PreparedFile | Symlink};

    // copy out specifies files need to be copied out from the container after execution
    // append '?' after file name will make the file optional and do not cause FileError when missing
    copyOut?: string[];
    // similar to copyOut but stores file in go judge and returns fileId, later download through /file/:fileId
    copyOutCached?: string[];
    // specifies the directory to dump container /w content
    copyOutDir: string
    // specifies the max file size to copy out
    copyOutMax?: number; // byte
}

enum Status {
    Accepted = 'Accepted', // normal
    MemoryLimitExceeded = 'Memory Limit Exceeded', // mle
    TimeLimitExceeded = 'Time Limit Exceeded', // tle
    OutputLimitExceeded = 'Output Limit Exceeded', // ole
    FileError = 'File Error', // fe
    NonzeroExitStatus = 'Nonzero Exit Status',
    Signalled = 'Signalled',
    InternalError = 'Internal Error', // system error
}

interface PipeIndex {
    index: number; // the index of cmd
    fd: number;    // the fd number of cmd
}

interface PipeMap {
    in: PipeIndex;  // input end of the pipe
    out: PipeIndex; // output end of the pipe
    // enable pipe proxy from in to out, 
    // content from in will be discarded if out closes
    proxy?: boolean; 
    name?: string;   // copy out proxy content if proxy enabled
    // limit the copy out content size, 
    // proxy will still functioning after max
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
    name: string; // error file name
    type: FileErrorType; // type
    message?: string; // detailed message
}

interface Request {
    requestId?: string; // for WebSocket requests
    cmd: Cmd[];
    pipeMapping?: PipeMap[];
}

interface CancelRequest {
    cancelRequestId: string;
};

// WebSocket request
type WSRequest = Request | CancelRequest;

interface Result {
    status: Status;
    error?: string; // potential system error message
    exitStatus: number;
    time: number;   // ns (cgroup recorded time)
    memory: number; // byte
    runTime: number; // ns (wall clock time)
    // copyFile name -> content
    files?: {[name:string]:string};
    // copyFileCached name -> fileId
    fileIds?: {[name:string]:string};
    // fileError contains detailed file errors
    fileError?: FileError[];
}

// WebSocket results
interface WSResult {
    requestId: string;
    results: Result[];
    error?: string;
}

// Stream request & responses
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

### Example Request & Response

<details><summary>FFI</summary>

```javascript
var ffi = require('ffi-napi');

var go_judge = ffi.Library('./go_judge', {
    'Init': ['int', ['string']],
    'Exec': ['string', ['string']],
    'FileList': ['string', []],
    'FileAdd': ['string', ['string']],
    'FileGet': ['string', ['string']],
    'FileDelete': ['string', ['string']]
});

if (go_judge.Init(JSON.stringify({
    cinitPath: "/judge/cinit",
    parallelism: 4,
}))) {
    console.log("Failed to init go judge");
}

const result = JSON.parse(go_judge.Exec(JSON.stringify({
    "cmd": [{
        "args": ["/bin/cat", "test.txt"],
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
            "test.txt": {
                "content": "TEST"
            }
        }
    }]
})));
console.log(result);

// Async
go_judge.Exec.async(JSON.stringify({
    "cmd": [{
        "args": ["/bin/cat", "test.txt"],
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
            "test.txt": {
                "content": "TEST"
            }
        }
    }]
}), (err, res) => {
    if (err) throw err;
    console.log(JSON.parse(res));
});

const fileAdd = (param) => new Promise((resolve, reject) => {
    go_judge.FileAdd.async(JSON.stringify(param), (err, res) => {
        if (err != null) { reject(err); } else { resolve(res); }
    });
});
const fileList = () => new Promise((resolve, reject) => {
    go_judge.FileList.async((err, res) => {
        if (err != null && res == null) { reject(err); } else { resolve(JSON.parse(res)); }
    });
});
const fileGet = (param) => new Promise((resolve, reject) => {
    go_judge.FileGet.async(JSON.stringify(param), (err, res) => {
        if (err != null && res == null) { reject(err); } else { resolve(res); }
    });
});
const fileDelete = (param) => new Promise((resolve, reject) => {
    go_judge.FileDelete.async(JSON.stringify(param), (err, res) => {
        if (err != null && res == null) { reject(err); } else { resolve(res); }
    });
});

const fileOps = async () => {
    const fileId = await fileAdd({ name: 'Name', content: 'Content' });
    console.log(fileId);
    const list = await fileList();
    console.log(list);
    const file = await fileGet({ id: fileId });
    console.log(file);
    const d = await fileDelete({ id: fileId });
    console.log(d);
    const e = await fileList();
    console.log(e);
};

fileOps();
```

Output:

```javascript
{
  requestId: '',
  results: [
    {
      status: 'Accepted',
      exitStatus: 0,
      time: 814048,
      memory: 253952,
      files: [Object]
    }
  ]
}
```

</details>

Please use PostMan or similar tools to send request to `http://localhost:5050/run`

<details><summary>Single (this example require `apt install g++` inside the container)</summary>

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
        "copyOutCached": ["a.cc", "a"]
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
            "a": "5LWIZAA45JHX4Y4Z",
            "a.cc": "NOHPGGDTYQUFRSLJ"
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
                "fileId": "5LWIZAA45JHX4Y4Z"
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

<details><summary>Multiple (interaction problem)</summary>

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

<details><summary>Compile On Windows (cygwin)</summary>

```json
{
    "cmd": [{
        "args": ["C:\\Cygwin\\bin\\g++", "a.cc", "-o", "a"],
        "env": ["PATH=C:\\Cygwin\\bin;"],
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
                "content": "#include <iostream>\n#include <signal.h>\n#include <unistd.h>\nusing namespace std;\nint main() {\nint a, b;\ncin >> a >> b;\ncout << a + b << endl;\n}"
            }
        },
        "copyOutCached": ["a.exe"]
    }]
}
```

```json
[
    {
        "status": "Accepted",
        "exitStatus": 0,
        "time": 140625000,
        "memory": 36286464,
        "files": {
            "stderr": "",
            "stdout": ""
        },
        "fileIds": {
            "a.exe": "HLQH2OF4MXUUJBCB"
        }
    }
]
```

</details>

<details><summary>Infinite loop with cpu rate control</summary>

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

### Command Line Arguments

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
- The default file store is in memory, local cache can be specified with `-dir` flag.
- The default CGroup prefix is `gojudge`, Can be specified with `-cgroup-prefix` flag.
- `-src-prefix` to restrict `src` copyIn path split by comma (need to be absolute path) (example: `/bin,/usr`)
- `-time-limit-checker-interval` specifies time limit checker interval (default 100ms) (valid value: \[1ms, 1s\])
- `-output-limit` specifies size limit of POSIX rlimit of output (default 256MiB)
- `-extra-memory-limit` specifies the additional memory limit to check memory limit exceeded (default 16KiB)
- `-copy-out-limit` specifies the default file copy out max (default 64MiB)
- `-open-file-limit` specifies the max number of open files (default 256)
- `-cpuset` specifies `cpuset.cpus` cgroup for each container (Linux only)
- `-container-cred-start` specifies container `setuid` / `setgid` credential start point (default: 0 (disabled)) (Linux only)
  - for example, by default container 0 will run with 10001 uid & gid and container 1 will run with 10002 uid & gid...
- `-enable-cpu-rate` enabled `cpu` cgroup to control cpu rate using cfs_quota & cfs_period control (Linux only)
  - `-cpu-cfs-period` specifies cfs_period if cpu rate is enabled (default 100ms) (valid value: \[1ms, 1s\])
- `-seccomp-conf` specifies `seccomp` filter setting to load when running program (need build tag `seccomp`) (Linux only)
  - for example, by `strace -c prog` to get all `syscall` needed and restrict to that sub set
  - however, the `syscall` count in one platform(e.g. x86_64) is not suitable for all platform, so this option is not recommended
  - the program killed by seccomp filter will have status `Dangerous Syscall`
- `-pre-fork` specifies number of container to create when server starts
- `-tmp-fs-param` specifies the tmpfs parameter for `/w` and `/tmp` when using default mounting (Linux only)
- `-file-timeout` specifies maximum TTL for file created in file store （e.g. `30m`)
- `-mount-conf` specifies detailed mount configuration, please refer `mount.yaml` as a reference (Linux only)
- `-container-init-path` specifies path to `cinit` (do not use, debug only) (Linux only)

### Environment Variables

Environment variable will be override by command line arguments if they both present and all command line arguments have its correspond environment variable (e.g. `ES_HTTP_ADDR`). Run `go-judge --help` to see all the environment variable configurations.

#### Build go judge

Build by your own `docker build -t go-judge -f Dockerfile.exec .`

For cgroup v1, the `go-judge` need root privilege to create `cgroup`. Either creates sub-directory `/sys/fs/cgroup/cpuacct/go_judge`, `/sys/fs/cgroup/memory/go_judge`, `/sys/fs/cgroup/pids/go_judge` and make execution user readable or use `sudo` to run it.

For cgroup v2, systemd dbus will be used to create a transient scope for cgroup integration.

#### Build Shared object

Build container init `cinit`:

`go build -o cinit ./cmd/go-judge-init`

Build `go_judge.so`:

`go build -buildmode=c-shared -o go_judge.so ./cmd/go-judge-ffi/`

For example, in JavaScript, run with `ffi-napi` (seems node 14 is not supported yet):

### Build gRPC Proxy

Build `go build ./cmd/go-judge-proxy`

Run `./go-judge-proxy`, connect to gRPC endpoint expose as a REST endpoint.

### Build go judge Shell

Build `go build ./cmd/go-judge-shell`

Run `./go-judge-shell`, connect to gRPC endpoint with interactive shell.

### Return Status

- Accepted: Program exited with status code 0 within time & memory limits
- Memory Limit Exceeded: Program uses more memory than memory limits
- Time Limit Exceeded:
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
- Signalled: Program exited with signal (e.g. SIGSEGV)
- Dangerous Syscall: Program killed by seccomp filter
- Internal Error:
  - Program is not exist
  - Or, container create not successful (e.g. not privileged docker)
  - Or, other errors

### Container Root Filesystem

For linux platform, the default mounts points are bind mounting host's `/lib`, `/lib64`, `/usr`, `/bin`, `/etc/ld.so.cache`, `/etc/alternatives`, `/etc/fpc.cfg`, `/dev/null`, `/dev/urandom`, `/dev/random`, `/dev/zero`, `/dev/full` and mounts tmpfs at `/w`, `/tmp` and creates `/proc`.

To customize mount points, please look at example `mount.yaml` file.

`tmpfs` size for `/w` and `/tmp` is configured through `-tmp-fs-param` with default value `size=128m,nr_inodes=4k`

If a file named `/.env` exists in the container rootfs, the container will load the file as environment variable line by line.

If a bind mount is specifying a target within the previous mounted one, please ensure the target exists in the previous mount point.

### Packages

- envexec: run single / group of programs in parallel within restricted environment and resource constraints
- env: reference implementation environments to inject into envexec

### Windows Support

- Build `go-judge` by: `go build ./cmd/go-judge/`
- Build `go_judge.dll`: (need to install `gcc` as well) `go build -buildmode=c-shared -o go_judge.so ./cmd/go-judge-ffi/`
- Run: `./go-judge`

#### Windows Security

- Resources are limited by [JobObject](https://docs.microsoft.com/en-us/windows/win32/procthread/job-objects)
- Privilege are limited by [Restricted Low Mandatory Level Token](https://docs.microsoft.com/en-us/windows/win32/secauthz/access-tokens)
- Low Mandatory Level directory is created for read / write

### MacOS Support

- Build `go-judge` by: `go build ./cmd/go-judge/`
- Build `go_judge.dylib`: (need to install `XCode`) `go build -buildmode=c-shared -o go_judge.dylib ./cmd/go-judge-ffi/`
- Run: `./go-judge`

#### MacOS Security

- `sandbox-init` profile deny network access and file read / write and read / write to `/Users` directory

### Notice

#### cgroup v2 support

The cgroup v2 is supported by `go-judge` now when running as root since more Linux distribution are enabling cgroup v2 by default (e.g. Ubuntu 21.10+, Fedora 31+). However, for kernel < 5.19, due to missing `memory.max_usage_in_bytes` in `memory` controller, the memory usage is now accounted by `maxrss` returned by `wait4` syscall. Thus, the memory usage appears higher than those who uses cgroup v1. For kernel >= 5.19, `memory.peak` is being used.

When running in containers, the `go-judge` will migrate all processed into `/api` hierarchy to enable nesting support.

When running in Linux distributions powered by `systemd`, the `go-judge` will contact `systemd` via `dbus` to create a transient scope as cgroup root.

#### Memory Usage

The controller will consume `20M` memory and each container will consume `20M` + size of tmpfs `2 * 128M`. For each request, it consumes as much as user program limit + extra limit (`16k`) + total copy out max. Notice that the cached file stores in the shared memory (`/dev/shm`) of the host, so please ensure enough size allocated.

For example, when concurrency = 4, the container itself can consume as much as `60 + (20+32) * 4M = 268M` + 4 * total copy out + total max memory of requests.

Due to limitation of GO runtime, the memory will not return to OS automatically, which could lead to OOM killer. The background worker was introduced to checks heap usage and invokes GC when necessary.

- `-force-gc-target` default `20m`, the minimal size to trigger GC
- `-force-gc-interval` default `5s`, the interval to check memory usage

### WebSocket Stream Interface

Websocket stream interface is used to run command interactively with inputs and outputs pumping from the command. All message is transmitted in binary format for maximum compatibility.

```text
+--------+--------+---...
| type   | payload ...
+--------|--------+---...
request:
type = 
  1 - request (payload = JSON encoded request)
  2 - resize (payload = JSON encoded resize request)
  3 - input (payload = 1 byte (4-bit index + 4-bit fd), followed by content)
  4 - cancel (no payload)

response:
type = 
  1 - response (payload = JSON encoded response)
  2 - output (payload = 1 byte (4-bit index + 4-bit fd), followed by content)
```

Any incomplete / invalid message will be treated as error.

### Benchmark

By `wrk` with `t.lua`: `wrk -s t.lua -c 1 -t 1 -d 30s --latency http://localhost:5050/run`.

However, these results are not the real use cases since the running time depends on the actual program specifies in the request. Normally, the go judge consumes ~1ms more compare to running without sandbox.

```lua
wrk.method = "POST"
wrk.body   = '{"cmd":[{"args":["/bin/cat","a.hs"],"env":["PATH=/usr/bin:/bin"],"files":[{"content":""},{"name":"stdout","max":10240},{"name":"stderr","max":10240}],"cpuLimit":10000000000,"memoryLimit":104857600,"procLimit":50,"copyIn":{"a.hs":{"content":"main = putStrLn \\"Hello, World!\\""},"b":{"content":"TEST"}}}]}'
wrk.headers["Content-Type"] = "application/json;charset=UTF-8"
```

- Single thread ~800-860 op/s Windows 10 WSL2 @ 5800X
- Multi thread ~4500-6000 op/s Windows 10 WSL2 @ 5800X

Single thread:

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

