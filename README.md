# go-judge

Under designing & development

[![GoDoc](https://godoc.org/github.com/criyle/go-judge?status.svg)](https://godoc.org/github.com/criyle/go-judge)

The goal to to reimplement [syzoj/judge-v3](https://github.com/syzoj/judge-v3) in GO language using [go-sandbox](https://github.com/criyle/go-sandbox).

## Planned Design

### Executor Service Draft (under development)

A rest service to run program in restricted environment and it is basically a wrapper for `pkg/envexec` to run single / multiple programs.

- /run POST execute program in the restricted environment
- /file GET list all cached file
- /file POST prepare a file in the executor service (in memory), returns fileId (can be referenced in /run parameter)
- /file/:fileId GET downloads file from executor service (in memory), returns file content
- /file/:fileId DELETE delete file specified by fileId

#### Install & Run Developing Server

Install GO 1.13+ from [download](https://golang.org/dl/)

```bash
go get github.com/criyle/go-judge/cmd/executorserver
~/go/bin/executorserver # or executorserver if $(GOPATH)/bin is in your $PATH
```

The `executorserver` need root privilege to create `cgroup`. Either creates sub-directory `/sys/fs/cgroup/cpuacct/go-judger`, `/sys/fs/cgroup/memory/go-judger`, `/sys/fs/cgroup/pids/go-judger` and make execution user readable or use `sudo` to run it.

The default binding address for the executor server is `:5050`. Can be specified with `-http` flag.

The default concurrency is `4`, Can be specified with `-parallism` flag.

The default file store is in memory, local cache can be specified wieh `-dir` flag.

#### Planed API interface

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

    // limitations
    cpuLimit?: number;     // s
    realCpuLimit?: number; // s
    memoryLimit?: number;  // byte
    procLimit?: number;

    // copy the correspond file to the container dst path
    copyIn?: {[dst:string]:LocalFile | MemoryFile | PreparedFile};

    // copy out specifies files need to be copied out from the container after execution
    copyOut?: string[];
    // similar to copyOut but stores file in executor service and returns fileId, later download through /file/:fileId
    copyOutCached?: string[];
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
    cmd: Cmd[];
    pipeMapping: PipeMap[];
}

interface Result {
    status: Status;
    error?: string; // potential system error message
    time: number;   // ns
    memory: number; // byte
    // copyFile name -> content
    files?: {[name:string]:string};
    // copyFileCached name -> fileId
    fileIds?: {[name:string]:string};
}
```

Example Request & Response:

Single:

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
		"cpuLimit": 10,
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
        "time": 303225231,
        "memory": 32243712,
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

Multiple:

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
		"cpuLimit": 1,
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
		"cpuLimit": 1,
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
        "time": 1545123,
        "memory": 253952,
        "files": {
            "stderr": ""
        },
        "fileIds": {}
    },
    {
        "status": "Accepted",
        "time": 1501463,
        "memory": 253952,
        "files": {
            "stderr": "",
            "stdout": "TEST 1"
        },
        "fileIds": {}
    }
]
```

### Workflow

``` text
    ^
    | Client (talk to the website)
    v
+------+    +----+
|      |<-->|Data|
|Judger|    +----+--+
|      |<-->|Problem|
+------+    +-------+
    ^
    | TaskQueue
    v
+------+   +--------+
|Runner|<->|Language|
+------+   +--------+
    ^
    | EnvExec
    v
+--------------------+
|ContainerEnvironment|
+--------------------+
```

### Container Root Filesystem

- [x] necessary lib / exec / compiler / header readonly bind mounted from current file system: /lib /lib64 /bin /usr
- [x] work directory tmpfs mount: /w (work dir), /tmp (compiler temp files)
- [ ] additional compiler scripts / exec readonly bind mounted: /c
- [ ] additional header readonly bind mounted: /i

### Interfaces

- client: receive judge tasks (websocket / socket.io / RabbitMQ / REST API)
- data: interface to download, cache, lock and access test data files from website (by dataId)
- taskqueue: message queue to send run task and receive run task result (In memory / (RabbitMQ, Redis))
- file: general file interface (disk / memory)
- language: programming language compile & execute configurations
- problem: parse problem definition from configuration files

### Judge Logic

- judger: execute judge logics (compile / standard / interactive / answer submit) and distribute as run task to queue, the collect and calculate results
- runner: receive run task and execute in sandbox environment

### Models

- JudgeTask: judge task pushed from website (type, source, data)
- JudgeResult: judge task result send back to website
- JudgeSetting: problem setting (from yaml) and JudgeCase
- RunTask: run task parameters send to run_queue
- RunResult: run task result sent back from queue

### Utilities

- pkg/envexec: run single / group of programs in parallel within restricted environment and resource constraints

## Planned API

### Progress

Client is able to report progress to the web front-end. Task should maintain its states

Planned events are:

- Parsed: problem data have been downloaded and problem configuration have been parsed (pass problem config to task)
- Compiled: user code have been compiled (success / fail)
- Progressed: single test case finished (success / fail - detail message)
- Finished: all test cases finished / compile failed

## TODO

- [x] socket.io client with namespace
- [x] judge_v3 protocol
- [ ] executor server
- [ ] syzoj problem YAML config parser
- [ ] syzoj data downloader
- [ ] syzoj compile configuration
- [ ] file io
- [ ] special judger
- [ ] interact problem
- [ ] answer submit
- [ ] demo site
