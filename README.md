# go-judge

[![Go Reference](https://pkg.go.dev/badge/github.com/criyle/go-judge.svg)](https://pkg.go.dev/github.com/criyle/go-judge) [![Go Report Card](https://goreportcard.com/badge/github.com/criyle/go-judge)](https://goreportcard.com/report/github.com/criyle/go-judge) [![Release](https://img.shields.io/github/v/tag/criyle/go-judge)](https://github.com/criyle/go-judge/releases/latest) ![Build](https://github.com/criyle/go-judge/workflows/Build/badge.svg)

## Executor Service

### Architecture

```text
+---------------------------------------------------------------------------+
| Transport Layer (HTTP / WebSocket / FFI / ...)                            |
+---------------------------------------------------------------------------+
| Executor Worker                                                           |
+-----------------------------------------------------------+---------------+
| Environment Pool + EnvExec + Environment Builder          | File Store    |
+--------------------+----------------+---------------------+--------+------+
| Linux (go-sandbox) | Windows (winc) | macOS (app sandbox) | Memory | Disk |
+--------------------+----------------+---------------------+--------+------+
```

### REST API

A REST service to run program in restricted environment and it is basically a wrapper for `envexec` to run single / multiple programs.

- /run POST execute program in the restricted environment (examples below)
- /file GET list all cached file
- /file POST prepare a file in the executor service (in memory), returns fileId (can be referenced in /run parameter)
- /file/:fileId GET downloads file from executor service (in memory), returns file content
- /file/:fileId DELETE delete file specified by fileId
- /ws WebSocket for /run
- /metrics prometheus metrics (specifies `ES_ENABLE_METRICS=1` environment variable to enable metrics)
- /debug (specifies `ES_ENABLE_DEBUG=1` environment variable to enable go runtime debug endpoint)
- /version gets build git version (e.g. `v0.9.4`) together with runtime information (go version, os, platform)

### Command Line Arguments

Server:

- The default binding address for the executor server is `:5050`. Can be specified with `-http-addr` flag.
- By default gRPC endpoint is disabled, to enable gRPC endpoint, add `-enable-grpc` flag.
- The default binding address for the gRPC executor server is `:5051`. Can be specified with `-grpc-addr` flag.
- The default log level is debug, use `-silent` to disable logs or use `-release` to enable release logger (auto turn on if in docker).
- `-auth-token` to add token-based authentication to REST / gRPC
- By default, the GO debug endpoints are disabled, to enable, specifies `-enable-debug`
- By default, the prometheus metrics endpoints are disabled, to enable, specifies `-enable-metrics`

Sandbox:

- The default concurrency is `4`, Can be specified with `-parallelism` flag.
- The default file store is in memory, local cache can be specified with `-dir` flag.
- The default CGroup prefix is `executor_server`, Can be specified with `-cgroup-prefix` flag.
- `-src-prefix` to restrict `src` copyIn path (need to be absolute path)
- `-time-limit-checker-interval` specifies time limit checker interval (default 100ms) (valid value: \[1ms, 1s\])
- `-output-limit` specifies size limit of POSIX rlimit of output (default 256MiB)
- `-extra-memory-limit` specifies the additional memory limit to check memory limit exceeded (default 16KiB)
- `-copy-out-limit` specifies the default file copy out max (default 64MiB)
- `-cpuset` specifies `cpuset.cpus` cgroup for each container
- `-container-cred-start` specifies container `setuid` / `setgid` credential start point (default: 10000)
  - for example, by default container 0 will run with 10001 uid & gid and container 1 will run with 10002 uid & gid...
- `-enable-cpu-rate` enabled `cpu` cgroup to control cpu rate using cfs_quota & cfs_period control
- `-cpu-cfs-period` specifies cfs_period if cpu rate is enabled (default 100ms) (valid value: \[1ms, 1s\])
- `-seccomp-conf` specifies `seecomp` filter setting to load when running program (need build tag `seccomp`)
  - for example, by `strace -c prog` to get all `syscall` needed and restrict to that sub set
  - however, the `syscall` count in one platform(e.g. x86_64) is not suitable for all platform, so this option is not recommended
  - the program killed by seccomp filter will have status `Dangerous Syscall`
- `-pre-fork` specifies number of container to create when server starts
- `-tmp-fs-param` specifies the tmpfs parameter for `/w` and `/tmp` when using default mounting

### Environment Variables

Environment variable will be override by command line arguments if they both present and all command line arguments have its correspond environment variable (e.g. `ES_HTTP_ADDR`). Run `executorserver --help` to see all the environment variable configurations.

### Install & Run

Download compiled executable from [Release](https://github.com/criyle/go-judge/releases) and run.

Or, by docker

```bash
docker run -it --rm --privileged -p 5050:5050 criyle/executorserver:demo
```

#### Build Executor Server

Build by your own `docker build -t executorserver -f Dockerfile.exec .`

The `executorserver` need root privilege to create `cgroup`. Either creates sub-directory `/sys/fs/cgroup/cpuacct/executor_server`, `/sys/fs/cgroup/memory/executor_server`, `/sys/fs/cgroup/pids/executor_server` and make execution user readable or use `sudo` to run it.

#### Build Shared object

Build container init `cinit`:

`go build -o cinit ./cmd/cinit`

Build `executor_server.so`:

`go build -buildmode=c-shared -o executor_server.so ./cmd/ffi/`

For example, in JavaScript, run with `ffi-napi` (seems node 14 is not supported yet):

### Build Executor Proxy

Build `go build ./cmd/executorproxy`

Run `./executorproxy`, connect to gRPC endpoint expose as a REST endpoint.

### Build Executor Shell

Build `go build ./cmd/executorshell`

Run `./executorshell`, connect to gRPC endpoint with interactive shell.

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

For linux platform, the default mounts points are bind mounting host's `/lib`, `/lib64`, `/usr`, `/bin`, `/etc/alternatives`, `/etc/fpc.cfg`, `/dev/null`, `/dev/urandom` and mounts tmpfs at `/w`, `/tmp` and creates `/proc`.

To customize mount points, please look at example `mount.yaml` file.

### Packages

- envexec: run single / group of programs in parallel within restricted environment and resource constraints
- env: reference implementation environments to inject into envexec

### Windows Support

- Build `executorserver` by: `go build ./cmd/executorserver/`
- Build `executor_server.dll`: (need to install `gcc` as well) `go build -buildmode=c-shared -o executor_server.so ./cmd/ffi/`
- Run: `./executorserver`

#### Windows Security

- Resources are limited by [JobObject](https://docs.microsoft.com/en-us/windows/win32/procthread/job-objects)
- Privillege are limited by [Restricted Low Mandatory Level Token](https://docs.microsoft.com/en-us/windows/win32/secauthz/access-tokens)
- Low Mandatory Level directory is created for read / write

### MacOS Support

- Build `executorserver` by: `go build ./cmd/executorserver/`
- Build `executor_server.dylib`: (need to install `XCode`) `go build -buildmode=c-shared -o executor_server.dylib ./cmd/ffi/`
- Run: `./executorserver`

#### MacOS Security

- `sandbox-init` profile deny network access and file read / write and read / write to `/Users` directory

### Notice

#### CentOS 7

By default, user namespace is disabled and it can be enabled following [stack overflow](https://superuser.com/questions/1294215/is-it-safe-to-enable-user-namespaces-in-centos-7-4-and-how-to-do-it/1294246#1294246)

```bash
echo user.max_user_namespaces=10000 >> /etc/sysctl.d/98-userns.conf
sysctl -p
```

### Benchmark

By `wrk` with `t.lua`: `wrk -s t.lua -c 1 -t 1 -d 30s --latency http://localhost:5050/run`.

However, these results are not the real use cases since the running time depends on the actual program specifies in the request. Normally, the executor server consumes ~1ms more compare to running without sandbox.

```lua
wrk.method = "POST"
wrk.body   = '{"cmd":[{"args":["/bin/cat","a.hs"],"env":["PATH=/usr/bin:/bin"],"files":[{"content":""},{"name":"stdout","max":10240},{"name":"stderr","max":10240}],"cpuLimit":10000000000,"memoryLimit":104857600,"procLimit":50,"copyIn":{"a.hs":{"content":"main = putStrLn \\"Hello, World!\\""},"b":{"content":"TEST"}}}]}'
wrk.headers["Content-Type"] = "application/json;charset=UTF-8"
```

- Single thread ~400-460 op/s Windows 10 WSL2
- Multi thread ~1100-1200 op/s Windows 10 WSL2

Single thread:

```text
Running 30s test @ http://localhost:5050/run
  1 threads and 1 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency     2.23ms  287.79us  14.29ms   86.51%
    Req/Sec   451.05     17.60   494.00     76.00%
  Latency Distribution
     50%    2.19ms
     75%    2.33ms
     90%    2.50ms
     99%    2.93ms
  13482 requests in 30.02s, 3.58MB read
Requests/sec:    449.15
Transfer/sec:    121.98KB
```

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

interface Pipe {
    name: string; // file name in copyOut
    max: number;  // maximum bytes to collect from pipe
}

interface Cmd {
    args: string[]; // command line argument
    env?: string[]; // environment

    // specifies file input / pipe collector for program file descriptors
    files?: (LocalFile | MemoryFile | PreparedFile | Pipe | null)[];
    tty?: boolean; // enables tty on the input and output pipes (should have just one input & one output)
    // Notice: must have TERM environment variables (e.g. TERM=xterm)

    // limitations
    cpuLimit?: number;     // ns
    realCpuLimit?: number; // deprecated: use clock limit instead (still working)
    clockLimit?: number;   // ns
    memoryLimit?: number;  // byte
    stackLimit?: number;   // byte (N/A on windows, macOS cannot set over 32M)
    procLimit?: number;
    strictMemoryLimit?: boolean; // Linux only: use stricter memory limit (+ rlimit_data when cgroup enabled)

    // copy the correspond file to the container dst path
    copyIn?: {[dst:string]:LocalFile | MemoryFile | PreparedFile};

    // copy out specifies files need to be copied out from the container after execution
    copyOut?: string[];
    // similar to copyOut but stores file in executor service and returns fileId, later download through /file/:fileId
    copyOutCached?: string[];
    // specifies the directory to dump container /w content
    copyOutDir: string
    // specifies the max file size to copy out
    copyOutMax: number; // byte
}

enum Status {
    Accepted,            // normal
    MemoryLimitExceeded, // mle
    TimeLimitExceeded,   // tle
    OutputLimitExceeded, // ole
    FileError,           // fe
    RuntimeError,        // re
    DangerousSyscall,    // dgs
    InternalError,       // system error
}

interface PipeIndex {
    index: number; // the index of cmd
    fd: number;    // the fd number of cmd
}

interface PipeMap {
    in: PipeIndex;  // input end of the pipe
    out: PipeIndex; // output end of the pipe
}

interface Request {
    requestId?: string; // for WebSocket requests
    cmd: Cmd[];
    pipeMapping: PipeMap[];
}

interface Result {
    status: Status;
    error?: string; // potential system error message
    time: number;   // ns (cgroup recorded time)
    memory: number; // byte
    runTime: number; // ns (wall clock time)
    // copyFile name -> content
    files?: {[name:string]:string};
    // copyFileCached name -> fileId
    fileIds?: {[name:string]:string};
}

// WebSocket results
interface WSResult {
    requestId: string;
    results: []Result;
    error?: string;
}
```

### Example Request & Response

FFI:

```javascript
var ffi = require('ffi-napi');

var executor_server = ffi.Library('./executor_server', {
    'Init': ['int', ['string']],
    'Exec': ['string', ['string']],
    'FileList': ['string', []],
    'FileAdd': ['string', ['string']],
    'FileGet': ['string', ['string']],
    'FileDelete': ['string', ['string']]
});

if (executor_server.Init(JSON.stringify({
    cinitPath: "/judge/cinit",
    parallelism: 4,
}))) {
    console.log("Failed to init executor server");
}

const result = JSON.parse(executor_server.Exec(JSON.stringify({
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
executor_server.Exec.async(JSON.stringify({
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
    executor_server.FileAdd.async(JSON.stringify(param), (err, res) => {
        if (err != null) { reject(err); } else { resolve(res); }
    });
});
const fileList = () => new Promise((resolve, reject) => {
    executor_server.FileList.async((err, res) => {
        if (err != null && res == null) { reject(err); } else { resolve(JSON.parse(res)); }
    });
});
const fileGet = (param) => new Promise((resolve, reject) => {
    executor_server.FileGet.async(JSON.stringify(param), (err, res) => {
        if (err != null && res == null) { reject(err); } else { resolve(res); }
    });
});
const fileDelete = (param) => new Promise((resolve, reject) => {
    executor_server.FileDelete.async(JSON.stringify(param), (err, res) => {
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

Single (this example require `apt install g++` inside the container):

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
        "copyOutCached": ["a.cc", "a"],
        "copyOutDir": "1"
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

Multiple (interaction problem):

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

Compile On Windows (cygwin):

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

Infinite loop with cpu rate control:

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
