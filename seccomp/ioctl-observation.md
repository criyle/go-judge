# Compiler and program ioctl observation

## Method

- Host: Linux, x86_64.
- Corpus: the 16 language configurations and sample programs in `go-judge-demo/src/constants/languageConfig.ts`.
- Coverage: each configured compile command and run command, 32 commands total.
- Tracing: `strace -f -qq -v -s 256 -e trace=ioctl`, so child compiler/runtime processes are included.
- Execution used pipes for standard input/output/error, not a PTY. The input was `1 2\n`.
- Counts below are from the original compile/run pass. Two environment adjustments were separately checked: TypeScript compilation passed after adding temporary ambient Node declarations; PHP produced the expected result when run with `short_open_tag=On`.

## Requests observed

| Request     | x86_64 request value | Calls | Argument / result notes                                                                                       |
| ----------- | -------------------: | ----: | ------------------------------------------------------------------------------------------------------------- |
| `TCGETS`    |             `0x5401` |   396 | Third argument was a pointer. All calls returned `ENOTTY` because the tested standard descriptors were pipes. |
| `TIOCGPGRP` |             `0x540f` |     2 | Third argument was a pointer. Both calls returned `ENOTTY`.                                                   |
| `FIONREAD`  |             `0x541b` |     2 | Third argument pointed to an integer; both calls returned `0`.                                                |
| `FIONBIO`   |             `0x5421` |     7 | Third argument values were `[1]` (5 calls) and `[0]` (2 calls); all returned success.                         |
| `TIOCSTI`   |             `0x5412` |     0 | Not observed in this corpus.                                                                                  |
| `TIOCLINUX` |             `0x541c` |     0 | Not observed in this corpus.                                                                                  |

## Calls by language and phase

Counts are calls, not unique argument pointers. `—` means no ioctl call was recorded.

| Language   | Compile                         | Run                          |
| ---------- | ------------------------------- | ---------------------------- |
| C          | `TCGETS` × 15                   | —                            |
| C++        | `TCGETS` × 19                   | —                            |
| Go         | —                               | —                            |
| JavaScript | —                               | `TCGETS` × 5; `FIONBIO` × 1  |
| TypeScript | `TCGETS` × 5; `FIONBIO` × 2     | `TCGETS` × 5; `FIONBIO` × 1  |
| Java       | `TCGETS` × 2                    | `TCGETS` × 2; `FIONREAD` × 2 |
| Pascal     | `TCGETS` × 10                   | —                            |
| Python     | `TCGETS` × 34                   | `TCGETS` × 13                |
| Haskell    | `TCGETS` × 105; `TIOCGPGRP` × 2 | —                            |
| Rust       | `TCGETS` × 23; `FIONBIO` × 3    | —                            |
| Ruby       | —                               | `TCGETS` × 128               |
| PHP        | —                               | `TCGETS` × 21                |
| C#         | `TCGETS` × 1                    | `TCGETS` × 1                 |
| Perl       | —                               | `TCGETS` × 4                 |
| Perl 6     | —                               | `TCGETS` × 3                 |
| OCaml      | —                               | —                            |

## Test notes

- The original TypeScript compile command exited 2 on this host because its global `tsc` installation could not resolve the `require` and `process` Node declarations. It still emitted JavaScript, and the run command exited 0. Repeating compilation with temporary ambient Node declarations made the exact compile command exit 0; the sample program printed `3`.
- The original PHP command exited 0, but this host has short PHP tags disabled, so the sample source was printed instead of executed. Running the same sample with `short_open_tag=On` printed `3` and exited 0.
- All other original compile and run commands exited 0; the arithmetic samples printed `3` (with expected newline differences).
- This traces the language samples directly on the Debian host. It does not exercise go-judge's seccomp filter or its PTY-enabled mode.
