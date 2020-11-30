# go-judge

[![GoDoc](https://godoc.org/github.com/criyle/go-judge?status.svg)](https://godoc.org/github.com/criyle/go-judge) [![Go Report Card](https://goreportcard.com/badge/github.com/criyle/go-judge)](https://goreportcard.com/report/github.com/criyle/go-judge) [![Release](https://img.shields.io/github/v/tag/criyle/go-judge)](https://github.com/criyle/go-judge/releases/latest) ![Build](https://github.com/criyle/go-judge/workflows/Build/badge.svg)

## Executor Service

### Architecture

#### Overall Architecture

```text
+----------------------------------------------------------------------------------+
| Transport Layer (HTTP / WebSocket / FFI / ...)                                   |
+----------------------------------------------------------------------------------+
| Executor Worker                                                                  |
+-----------------------------------------------------------+----------------------+
| EnvExec + Environment Pool + Environment Builder          | File Store           |
+--------------------+----------------+---------------------+--------+-------+-----+
| Linux (go-sandbox) | Windows (winc) | macOS (app sandbox) | Memory | Local | ... |
+--------------------+----------------+---------------------+--------+-------+-----+
```

A rest service to run program in restricted environment and it is basically a wrapper for `pkg/envexec` to run single / multiple programs.

- /run POST execute program in the restricted environment
- /file GET list all cached file
- /file POST prepare a file in the executor service (in memory), returns fileId (can be referenced in /run parameter)
- /file/:fileId GET downloads file from executor service (in memory), returns file content
- /file/:fileId DELETE delete file specified by fileId
- /ws WebSocket for /run
- /metrics prometheus metrics (specifies `METRICS=1` environment variable to enable metrics)
- /debug (specifies `DEBUG=1` environment variable to enable go runtime debug endpoint)
- /version gets build git version (e.g. `v0.6.4-1-g20d2815`) together with runtime information (go version, os, platform)

### Install & Run Developing Server

Install GO 1.13+ from [download](https://golang.org/dl/)

```bash
go get github.com/criyle/go-judge/cmd/executorserver
~/go/bin/executorserver # or executorserver if $(GOPATH)/bin is in your $PATH
```

Or, by docker

```bash
docker run -it --rm --privileged -p 5050:5050 criyle/executorserver:demo
```

Build by your own `docker build -t executorserver -f Dockerfile.exec .`

The `executorserver` need root privilege to create `cgroup`. Either creates sub-directory `/sys/fs/cgroup/cpuacct/executor_server`, `/sys/fs/cgroup/memory/executor_server`, `/sys/fs/cgroup/pids/executor_server` and make execution user readable or use `sudo` to run it.

#### Command Line Arguments

- The default binding address for the executor server is `:5050`. Can be specified with `-http-addr` flag.
- The default binding address for the gRPC executor server is `:5051`. Can be specified with `-grpc-addr` flag. (Notice: need to set `ES_ENABLE_GRPC=1` environment variable to enable GRPC endpoint)
- The default concurrency is `4`, Can be specified with `-parallelism` flag.
- The default file store is in memory, local cache can be specified with `-dir` flag.
- The default log level is debug, use `-silent` to disable logs or use `-release` to enable release logger (auto turn on if in docker).
- The default CGroup prefix is `executor_server`, Can be specified with `-cgroup-prefix` flag.
- `-auth-token` to add token-based authentication to REST / gRPC
- `-src-prefix` to restrict `src` copyIn path (need to be absolute path)
- `-time-limit-checker-interval` specifies time limit checker interval (default 100ms)

#### Environment Variables

Environment variable will be override by command line arguments if they both present. All command line arguments have its correspond environment variable.

- The http binding address specifies as `ES_HTTP_ADDR=addr`
- The grpc binding address specifies as `ES_GRPC_ADDR=addr`
- The parallelism specifies as `ES_PARALLELISM=4`
- The token specifies as `ES_AUTH_TOKEN=token`
- `ES_ENABLE_GRPC=1` enables gRPC
- `ES_ENABLE_METRICS=1` enables metrics
- `ES_ENABLE_DEBUG=1` enables debug

### Build Shared object

Build container init `cinit`:

`go build -o cinit ./cmd/cinit`

Build `executor_server.so`:

`go build -buildmode=c-shared -o executor_server.so ./cmd/ffi/`

For example, in JavaScript, run with `ffi-napi` (seems node 14 is not supported yet):

### Build Executor Proxy

Build `go build ./cmd/executorproxy`

Run `./executorproxy`, connect to gRPC endpoint and offers REST endpoint.

### Build Executor Shell

Build `go build ./cmd/executorshell`

Run `./executorshell`, connect to gRPC endpoint with interactive shell.

### Container Root Filesystem

- [x] necessary lib / exec / compiler / header readonly bind mounted from current file system: /lib /lib64 /bin /usr
- [x] work directory tmpfs mount: /w (work dir), /tmp (compiler temp files)

The following mounts point are examples that can be configured through config file later

- additional compiler scripts / exec readonly bind mounted: /c
- additional header readonly bind mounted: /i

### Utilities

- pkg/envexec: run single / group of programs in parallel within restricted environment and resource constraints
- pkg/pool: reference implementation for Cgroup & Environment Pool

### Windows Support

Build `executorserver` by:

`go build ./cmd/executorserver/`

Build `executor_server.dll`: (need to install `gcc` as well)

`go build -buildmode=c-shared -o executor_server.so ./cmd/ffi/`

Run: `./executorserver`

#### Windows Security

- Resources are limited by [JobObject](https://docs.microsoft.com/en-us/windows/win32/procthread/job-objects)
- Privillege are limited by [Restricted Low Mandatory Level Token](https://docs.microsoft.com/en-us/windows/win32/secauthz/access-tokens)
- Low Mandatory Level directory is created for read / write

### MacOS Support

Build `executorserver` by:

`go build ./cmd/executorserver/`

Build `executor_server.dylib`: (need to install `XCode`)

`go build -buildmode=c-shared -o executor_server.dylib ./cmd/ffi/`

Run: `./executorserver`

#### MacOS Security

- `sandbox-init` profile deny network access and file read / write

### Benchmark

By `wrk` with `t.lua`: 

- Tested single thread ~140-160 op/s macOS Docker Desktop & ~400-460 op/s Windows 10 WSL2.
- Tested multi thread ~1100-1200 op/s Windows 10 WSL2

```lua
wrk.method = "POST"
wrk.body   = '{"cmd":[{"args":["/bin/cat","a.hs"],"env":["PATH=/usr/bin:/bin"],"files":[{"content":""},{"name":"stdout","max":10240},{"name":"stderr","max":10240}],"cpuLimit":10000000000,"memoryLimit":104857600,"procLimit":50,"copyIn":{"a.hs":{"content":"main = putStrLn \\"Hello, World!\\""},"b":{"content":"TEST"}}}]}'
wrk.headers["Content-Type"] = "application/json;charset=UTF-8"
```

`wrk -s t.lua -c 1 -t 1 -d 30s --latency http://localhost:5050/run`

e.g.:

```text
Running 30s test @ http://localhost:5050/run
  1 threads and 1 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency     2.17ms  446.05us  22.37ms   93.88%
    Req/Sec   463.26     26.15   500.00     78.67%
  Latency Distribution
     50%    2.08ms
     75%    2.27ms
     90%    2.50ms
     99%    3.13ms
  13846 requests in 30.01s, 3.65MB read
Requests/sec:    461.30
Transfer/sec:    124.38KB
```

## TODO

- [x] Github actions to auto build
- [x] Configure mounts using YAML config file
- [x] Investigate root-free running mechanism (no cgroup && not set uid / gid)
- [x] Investigate RLimit settings (cpu, data, fsize, stack, noFile)
- [x] Add WebSocket for job submission
- [x] Windows support
- [x] MacOS support
- [x] GRPC + protobuf support
- [x] Token-based authentication
- [x] Prometheus metrics support
- [x] Customize container workDir, hostName & domainName

## API interface

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
    realCpuLimit?: number; // ns
    memoryLimit?: number;  // byte
    stackLimit?: number;   // byte (N/A on windows, macOS cannot set over 32M)
    procLimit?: number;

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
