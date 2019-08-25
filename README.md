# go-judge

Under designing.

The goal to to reimplement [syzoj/judge-v3](https://github.com/syzoj/judge-v3) in GO language using [go-sandbox](https://github.com/criyle/go-sandbox).

## Planned Design

Brokers and interfaces:

+ client: receive pushed judge tasks from website (web-socket / socket.io / RabbitMQ)
+ run_queue: send and receive run task to a queue (GO channel / RabbitMQ)
+ test_data: download, cache and access files from website
+ exec_files: upload / download executable files from compile task (local / Redis)

Workers:

+ judger: execute judge tasks and distribute as run task to queue
+ runner: receive run task and execute in sandbox

Models:

+ JudgeTask: judge task pushed from website (type, data)
+ JudgeSetting: problem setting (from yaml) and JudgeCase
+ RunTask: run task parameters send to run_queue
+ RunResult: run task result sent back from queue
+ Language: defines exec parameters

Utilities:

+ Config: read client config from TOML file
