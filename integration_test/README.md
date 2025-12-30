# Integration Tests

To run integration test, you need to have a running `go-judge` instance in local environment and running with

```sh
go test -tags integration -v -count=1 .
```

To run benchmark test

```sh
# -cpu 1 is necessary to control the parallelism
go test -tags integration -cpu 1 -bench . 
```
