# go-judge

Under designing.

The goal to to reimplement [syzoj/judge-v3](https://github.com/syzoj/judge-v3) in GO language using [go-sandbox](https://github.com/criyle/go-sandbox).

## Planned Design

Container Root Filesystem:

+ necessary lib / exec / compiler / header readonly bind mounted from current file system: /lib /lib64 /bin /usr
+ work directory tmpfs mount: /w (work dir), /tmp (compiler temp files)
+ additional compiler scripts / exec readonly bind mounted: /c
+ additional header readonly bind mounted: /i

Brokers and interfaces:

+ client: receive pushed judge tasks from website (websocket / socket.io / RabbitMQ)
+ data: interface to download, cache, lock and access test data files from website by data_id
+ taskqueue: message queue to send run task and receive result (GO channel / (RabbitMQ, Redis))
+ file: general file interface (disk / memory)
+ language: programming language compile & execute configurations

Workers:

+ judger: execute judge logics (compile / standard / interactive / answer submit) and distribute as run task to queue
+ runner: receive run task and execute in sandbox (dumb runner)

Models:

+ JudgeTask: judge task pushed from website (type, source, data)
+ JudgeResult: judge task result send back to website
+ JudgeSetting: problem setting (from yaml) and JudgeCase
+ RunTask: run task parameters send to run_queue
+ RunResult: run task result sent back from queue

Utilities:

+ Config: read client config from TOML file
+ pkg/runner: run a group of programs in parallel

## TODO

+ judge_v3 protocol socket.io client
+ syzoj problem yml parser
+ syzoj compile configuration
+ special differ
+ interact problem support
+ answer submit problem support
