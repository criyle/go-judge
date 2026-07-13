# Stream turn control

Turn control extends `/stream` for long-running, turn-based programs. It is
optional: `/run` and `/stream` sessions that never send a control message keep
their existing resource-limit and output behavior.

## Requirements

- Linux with a unified cgroup v2 hierarchy and a writable `cgroup.freeze` file.
- The normal go-judge cgroup pool must be enabled. cgroup v1 and non-cgroup
  environments return a `controlError` whose message states that turn control
  is unsupported.
- Each controlled action is written to one `streamOut` file descriptor. Debug
  output belongs on stderr or another non-controlled descriptor.

## WebSocket protocol

The first frame remains the existing type `1` execution request. Control
requests and responses use binary frame type `5`; the remaining bytes contain
JSON.

Begin turn request:

```json
{
  "index": 0,
  "beginTurn": {
    "turnId": 17,
    "moveCpuLimit": 200000000,
    "totalCpuLimit": 10000000000,
    "wallLimit": 2000000000,
    "outputFd": 1,
    "delimiter": "\n",
    "maxOutput": 4096
  }
}
```

Successful response:

```json
{
  "requestId": "game-42",
  "index": 0,
  "turnId": 17,
  "type": "turnCompleted",
  "moveCpu": 18340291,
  "totalCpu": 917401033,
  "wallTime": 20839127,
  "output": "MOVE 1 2\n"
}
```

gRPC uses `StreamRequest.execControl` and `StreamResponse.execControl` with the
same fields. All durations are nanoseconds.

Event types are `turnCompleted`, `moveCpuLimitExceeded`,
`totalCpuLimitExceeded`, `moveWallLimitExceeded`,
`turnOutputLimitExceeded`, `processExited`, and `controlError`.

## Lifecycle and timing

On a begin request, go-judge freezes every command in the execution group,
records the selected command's CPU baseline, clears its turn buffer, and then
resumes only that command. A second turn cannot begin while any action is
active.

The controlled output is buffered locally until the delimiter is found. The
process cgroup is frozen and the final CPU usage is read before the event is
sent to the client. Bytes after the first delimiter are not part of the action.
If the delimiter has not completed before `maxOutput`, the action ends with
`turnOutputLimitExceeded`.

CPU usage comes from `envexec.Process.Usage`, which reads the command's cgroup
and includes its main process, threads, and descendants. Initialization CPU is
included in `totalCpu`. Limits use a strict greater-than comparison:

```text
moveCpu  > moveCpuLimit
totalCpu > totalCpuLimit
wallTime > wallLimit
```

Active turns use a 1 ms polling interval. The minimum non-zero move CPU limit
is 50 ms; ordinary executions retain their configured interval (100 ms by
default). CPU accounting on cgroup v2 is reported in microseconds and exposed
as nanoseconds, so results are not microsecond hard-real-time. Overshoot can
include one polling interval plus scheduler and cgroup-freeze latency. The
delimiter path avoids network round-trip latency by freezing locally.

Fatal control events cancel the entire execution group. The control event is
the authoritative game decision; the final ordinary result only records the
subsequent process exit. Client disconnect and explicit cancel use the existing
execution context cleanup path.

## Compatibility

- `/run` does not create runtime controllers and is unchanged.
- `/stream` without type `5` messages continues forwarding `streamOut` bytes.
- Controlled bytes on the selected descriptor are delivered only in the
  control event, not as ordinary output frames.
- Existing WebSocket frame types `1` through `4` and existing protobuf field
  numbers are unchanged.

