# Two-AI turn-control demo

Build one binary and copy it twice into a go-judge request:

```sh
g++ -O2 -std=c++17 ai.cpp -o ai
```

Start a `/stream` execution with two long-running commands. Give each command a
`streamIn` fd 0 and `streamOut` fd 1. Then alternate type-5 begin-turn frames
and type-3 input frames:

1. Begin AI 0 with a 200 ms move limit and send `normal\n`: the response is
   `turnCompleted`.
2. Begin AI 1 with a 100 ms move limit and send `move-timeout\n`: the response
   is `moveCpuLimitExceeded`.
3. In a new match, set `totalCpuLimit` to 150 ms and send `total-step\n` on two
   turns of the same AI: the second response is `totalCpuLimitExceeded`.

Use a wall limit of at least one second and `delimiter: "\n"`, `maxOutput:
4096`. The complete request and event schemas are documented in
`docs/turn-control.md`.

`driver.go` automates the normal and cumulative cases against a local server:

```sh
go run driver.go "$PWD/ai"
```

To demonstrate the single-move timeout, change either normal input in
`driver.go` to `move-timeout\n` and set that call's move limit to 100 ms. A
fatal timeout ends the execution group, so run it separately from the
cumulative example.
