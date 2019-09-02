# go-judge

Under designing.

The goal to to reimplement [syzoj/judge-v3](https://github.com/syzoj/judge-v3) in GO language using [go-sandbox](https://github.com/criyle/go-sandbox).

## Planned Design

Brokers and interfaces:

+ client: receive pushed judge tasks from website (web-socket / socket.io / RabbitMQ)
+ data: interface to download, cache and access test files from website by id
+ taskqueue: send to and receive from a queue to run task (GO channel / (RabbitMQ, Redis)) and also including upload / download executable files from compile task
+ file: general file interface (local / memory)
+ language: language configuration for runner

Workers:

+ judger: execute judge tasks and distribute as run task to queue
+ runner: receive run task and execute in sandbox (compile / standard / interactive / answer submit)

Models:

+ JudgeTask: judge task pushed from website (type, data)
+ JudgeSetting: problem setting (from yaml) and JudgeCase
+ RunTask: run task parameters send to run_queue
+ RunResult: run task result sent back from queue

Utilities:

+ Config: read client config from TOML file
