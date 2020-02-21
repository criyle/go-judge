# SYZOJ judge-v3 Protocol

## Overall

Transmission protocol: socket.io
Encoding: msgpack encode

Connect: socket.io connect to syzoj.domain/socket.io/ namespace: judge

Queue (Infinite Loop):

+ C -> S: waitForTask `token`
+ S -> C (wait) (*): onTask `task (msgpack)`
+ Progress
  + C -> S: reportProgress `token` `progress (msgpack)`
  + C -> S: reportResult `token` `result (msgpack)`
+ C -> S: ack (*)

Progress:

+ Started progress
+ Compiled progress / result
  + If compile success, foreach test cases
  + Progress progress
+ Finished progress / result

Exceptions:

+ S onDisconnect: requeue task
+ S invalid token: no-op

## Data Structures

### token

judge_token in config.json

### enums

ProblemType

``` text
1 - Standard
2 - AnswerSubmission
3 - Interaction
```

ProgressReportType

``` text
1 - Started
2 - Compiled
3 - Progress
4 - Finished
5 - Reported
```

TaskStatus

``` text
0 - Waiting
1 - Running
2 - Done
3 - Failed
4 - Skipped
```

TaskStatus Transition

``` text
Waiting
    -> Running
        -> Done
        -> Failed
    -> Skipped
```

TestcaseResultType

``` typescript
enum TestcaseResultType {
    Accepted = 1,
    WrongAnswer = 2,
    PartiallyCorrect = 3,
    MemoryLimitExceeded = 4,
    TimeLimitExceeded = 5,
    OutputLimitExceeded = 6,
    FileError = 7, // The output file does not exist
    RuntimeError = 8,
    JudgementFailed = 9, // Special Judge or Interactor fails
    InvalidInteraction = 10
}
```

### task

``` typescript
interface {
    content: {
        taskId: string;
        testData: string;
        type: ProblemType;
        priority: number;
        param: {
            // shared
            language: string;
            code: string;
            timeLimit: number;    // ms
            memoryLimit: number;  // mb

            // standard
            fileIOInput?: string;  // Null indicates stdio.
            fileIOOutput?: string;
        }
    };
    // answer submission
    extraData?: Buffer;
}
```

### progress / result (ProgressReportData)

``` typescript
interface {
    taskId: string;
    type: ProgressReportType;
    progress: {
        // compile
        status: TaskStatus;
        message: string;

        // overall
        error?: SystemError | TestDataError; // enum 0 / 1
        systemMessage?: string;
        compile?: CompilationResult
        judge?: {
            subtasks?: []{ // subtask result
                score?: number;
                cases: []{ // test case result
                    status: TaskStatus;
                    result?: TestcaseDetails;
                    errorMessage?: string;
                }
            }
        }
    }
}
```

CompilationResult

``` typescript
interface {
    status: TaskStatus;
    message?: string;
}
```

TestcaseDetails

``` typescript
interface {
    type: TestcaseResultType;
    time: number; // ms
    memory: number; // kb
    input?: FileContent;
    output?: FileContent; // Output in test data
    scoringRate: number; // e.g. 0.5
    userOutput?: string;
    userError?: string;
    spjMessage?: string;
    systemMessage?: string;
}
```

FileContent

``` typescript
interface FileContent {
    content: string,
    name: string
}
```
