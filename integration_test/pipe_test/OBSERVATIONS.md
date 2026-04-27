# Pipe Proxy Performance Observations

This note summarizes the proxy performance experiments added during investigation of the `cpuset` / proxy regression.

It covers two different test shapes:

1. in-process unit benchmarks in `go-judge/envexec`
2. cross-process integration runs in `go-judge/integration_test/pipe_test`

The distinction matters:

- the unit benchmarks isolate pipe/relay behavior inside one Go process
- the integration runs exercise the real multi-process IPC shape that `go-judge` cares about

## Test Shapes

### In-process unit benchmarks

Files:

- [file_pipe_test.go](/home/criyle/project/judge/go-judge/envexec/file_pipe_test.go)
- [file_pipe_linux_test.go](/home/criyle/project/judge/go-judge/envexec/file_pipe_linux_test.go)

Relevant benchmarks:

- `BenchmarkInteractiveProxy`
- `BenchmarkProxyAffinity`
- `BenchmarkProxyBidirectionalAffinity`

These benchmarks pin Go threads with `sched_setaffinity` and compare:

- direct copy relay (`StdProxy`)
- `splice` relay (`ZeroCopy`)
- different message sizes
- different CPU placements

### Cross-process integration harness

File:

- [test.go](/home/criyle/project/judge/go-judge/integration_test/pipe_test/test.go)

The harness runs two Python processes in a ping-pong loop and supports:

- `none`: direct pipe baseline, no proxy
- `std`: parent-side `io.Copy` relay
- `splice`: parent-side `unix.Splice` relay

It also supports CPU placement for:

- producer process
- consumer process
- `A -> B` relay thread
- `B -> A` relay thread

Layouts:

- `all-same`
- `proc-same-relay-other`
- `all-split`

## Commands Used

### Unit benchmarks

Interactive proxy benchmark:

```bash
env GOCACHE=/tmp/go-cache go test -run '^$' -bench '^BenchmarkInteractiveProxy$' -benchtime=2s ./go-judge/envexec
```

Affinity matrix benchmarks:

```bash
env GOCACHE=/tmp/go-cache go test -run '^$' -bench '^(BenchmarkProxyAffinity|BenchmarkProxyBidirectionalAffinity)$' -benchtime=200ms ./go-judge/envexec
```

### Cross-process integration run

```bash
env GOCACHE=/tmp/go-cache go run -tags=integration ./go-judge/integration_test/pipe_test -mode all -layout all -n 1 -p 1
```

## Observations

### 1. In-process microbenchmarks do not predict the real IPC result by themselves

`BenchmarkInteractiveProxy` showed that for the in-process ping-pong benchmark, `StdProxy` beat `ZeroCopy` by a large margin for very small messages.

Observed examples:

- `StdProxy/Msg-8B`: about `11.4 us/op`
- `ZeroCopy/Msg-8B`: about `93.9 us/op`
- `StdProxy/Msg-1024B`: about `11.5 us/op`
- `ZeroCopy/Msg-1024B`: about `97.0 us/op`

This showed that `tee`/`splice` is not automatically better than `read`/`write` in a local benchmark with tiny interactive messages.

However, that result did **not** carry over directly to the real multi-process proxy setup.

### 2. Direct cross-process pipes are highly sensitive to process placement

Integration baseline (`none`) results:

- `none/all-same`: `1.57s`
- `none/proc-same-relay-other`: `1.58s`
- `none/all-split`: `7.65s`

Interpretation:

- when there is no proxy, relay placement is irrelevant
- splitting the two communicating processes across CPUs is very expensive for this ping-pong workload
- direct IPC on split CPUs was about `4.9x` slower than direct IPC on the same CPU

This confirms the original observation that locality matters a lot for this workload.

### 3. The std proxy is consistently slow in the real multi-process case

Integration `std` results:

- `std/all-same`: `13.78s`
- `std/proc-same-relay-other`: `16.37s`
- `std/all-split`: `16.97s`

Interpretation:

- user-space relay overhead dominates
- moving relays away from the communicating processes makes it somewhat worse
- but even the best `std` case is still far slower than direct pipes

Relative to direct baseline on the same layout:

- `std/all-same` vs `none/all-same`: about `8.8x` slower

### 4. The splice proxy can be good, but only with strong locality

Integration `splice` results:

- `splice/all-same`: `3.05s`
- `splice/proc-same-relay-other`: `13.10s`
- `splice/all-split`: `14.44s`

Interpretation:

- when producer, consumer, and both relay threads stay together, `splice` performs much better than `std`
- when relay threads move away from the communicating processes, `splice` degrades sharply

Relative comparisons:

- `splice/all-same` vs `std/all-same`: about `4.5x` faster
- `splice/all-same` vs `none/all-same`: about `1.9x` slower
- `splice/proc-same-relay-other` vs `splice/all-same`: about `4.3x` slower

This was the clearest evidence that the main problem is not simply “proxy is expensive”, but “proxy locality is critical”.

### 5. Affinity effects differ between the in-process and cross-process setups

The unit affinity benchmarks showed mixed behavior:

- in some one-way cases, splitting roles improved throughput
- in bidirectional in-process cases, `ZeroCopy` improved when roles were spread out

That is useful for understanding raw relay mechanics, but it is **not** enough to pick the production strategy.

The integration harness is more relevant because it includes:

- real process scheduling
- parent/child IPC
- actual ping-pong behavior
- actual parent-side relay threads

For implementation decisions, the integration results should carry more weight than the in-process microbenchmarks.

## Practical Conclusion

The current evidence supports the following conclusions:

1. The direct pipe baseline benefits heavily from keeping the communicating processes on the same CPU or very local CPUs.
2. The `std` proxy is consistently poor for this workload and should not be the main optimization target unless `splice` cannot be used.
3. The `splice` proxy is viable for this workload, but only if relay thread affinity preserves locality.
4. An unpinned parent-side relay can destroy most of the cpuset win, especially for the `splice` path.

## Implementation Direction

The most promising implementation direction is:

1. Keep the proxy semantics, since it also provides the drain/anti-`SIGPIPE` guardrail.
2. Prefer the `splice` proxy for this ping-pong workload.
3. Pin proxy relay goroutines/threads to the same CPU or same `cpuset` as the communicating tasks.
4. Re-run the integration matrix after implementing relay affinity to verify that `splice/all-same`-like behavior is preserved in the actual `go-judge` execution path.

## Caveats

- The integration matrix above was collected with `-n 1`, so exact timings should not be treated as final.
- The ratios are large enough to be meaningful, but future work should repeat with `-n 3` or `-n 5`.
- The current integration harness does not yet sweep payload sizes. It focuses on the interactive ping-pong shape that most closely matches the observed regression.
