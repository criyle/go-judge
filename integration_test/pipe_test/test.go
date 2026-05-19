//go:build integration && linux

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

const iterCount = 100000

const producerScript = `
import sys
import time

sys.stdout.reconfigure(line_buffering=True)

sys.stdout.write("PING\n")
sys.stdout.flush()
reply = sys.stdin.readline()
if not reply:
    sys.stderr.write("Producer: Peer disconnected immediately\n")
    sys.exit(1)

start = time.time()
for i in range(%d):
    sys.stdout.write(f"{i}\n")
    _ = sys.stdin.readline()

duration = time.time() - start
sys.stderr.write(f"Producer: Done {i+1} iters in {duration:.4f}s\n")
`

const consumerScript = `
import sys

sys.stdout.reconfigure(line_buffering=True)

while True:
    line = sys.stdin.readline()
    if not line:
        break
    sys.stdout.write(line)
`

type relayMode struct {
	name     string
	proxy    bool
	zeroCopy bool
}

type placement struct {
	name  string
	aCPU  int
	bCPU  int
	abCPU int
	baCPU int
}

var (
	modes = []relayMode{
		{name: "none", proxy: false, zeroCopy: false},
		{name: "std", proxy: true, zeroCopy: false},
		{name: "splice", proxy: true, zeroCopy: true},
	}
	placements = []placement{
		{name: "all-same", aCPU: 0, bCPU: 0, abCPU: 0, baCPU: 0},
		{name: "proc-same-relay-other", aCPU: 0, bCPU: 0, abCPU: 1, baCPU: 1},
		{name: "all-split", aCPU: 0, bCPU: 1, abCPU: 2, baCPU: 3},
	}
)

func main() {
	parallelism := flag.Int("p", 1, "number of parallel executions")
	totalRuns := flag.Int("n", 1, "total runs for each mode/layout pair")
	modeFilter := flag.String("mode", "all", "relay mode: all|none|std|splice")
	layoutFilter := flag.String("layout", "all", "layout: all|all-same|proc-same-relay-other|all-split")
	flag.Parse()

	if runtime.NumCPU() < 4 {
		panic("requires at least 4 CPUs")
	}

	tmpDir, err := os.MkdirTemp("", "pipe_repro")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	prodPath := filepath.Join(tmpDir, "producer.py")
	consPath := filepath.Join(tmpDir, "consumer.py")

	if err := os.WriteFile(prodPath, []byte(fmt.Sprintf(producerScript, iterCount)), 0755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(consPath, []byte(consumerScript), 0755); err != nil {
		panic(err)
	}

	selectedModes := filterModes(*modeFilter)
	selectedPlacements := filterPlacements(*layoutFilter)

	fmt.Printf("--- Starting Proxy IPC Test ---\n")
	fmt.Printf("Scripts prepared in: %s\n", tmpDir)
	fmt.Printf("Workload: %d iterations per process\n", iterCount)
	fmt.Printf("Parallelism: %d\n", *parallelism)
	fmt.Printf("Runs per pair: %d\n", *totalRuns)
	fmt.Printf("Modes: %v\n", modeNames(selectedModes))
	fmt.Printf("Layouts: %v\n\n", placementNames(selectedPlacements))

	var globalWG sync.WaitGroup
	sem := make(chan struct{}, *parallelism)
	globalStart := time.Now()

	for _, mode := range selectedModes {
		for _, layout := range selectedPlacements {
			label := fmt.Sprintf("%s/%s", mode.name, layout.name)
			var successCount int64
			var failCount int64
			start := time.Now()

			for i := 0; i < *totalRuns; i++ {
				globalWG.Add(1)
				sem <- struct{}{}
				go func(id int, mode relayMode, layout placement) {
					defer globalWG.Done()
					defer func() { <-sem }()

					if err := runInstance(id, prodPath, consPath, mode, layout); err != nil {
						fmt.Printf("[%s][ID %d] FAILED: %v\n", label, id, err)
						atomic.AddInt64(&failCount, 1)
						return
					}
					atomic.AddInt64(&successCount, 1)
				}(i, mode, layout)
			}
			globalWG.Wait()

			fmt.Printf("[%s] Summary | Total: %v | Success: %d | Failed: %d\n",
				label, time.Since(start), successCount, failCount)
		}
	}

	fmt.Printf("\n--- Overall Summary ---\n")
	fmt.Printf("Total Time: %v\n", time.Since(globalStart))
}

func runInstance(id int, prodPath, consPath string, mode relayMode, layout placement) error {
	abSrcR, abSrcW, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("ab source pipe: %w", err)
	}
	baSrcR, baSrcW, err := os.Pipe()
	if err != nil {
		closeAll(abSrcR, abSrcW)
		return fmt.Errorf("ba source pipe: %w", err)
	}

	abDstR, abDstW := abSrcR, abSrcW
	baDstR, baDstW := baSrcR, baSrcW
	var parentToClose []*os.File
	var abRelayDone <-chan error
	var baRelayDone <-chan error

	if mode.proxy {
		abDstR, abDstW, err = os.Pipe()
		if err != nil {
			closeAll(abSrcR, abSrcW, baSrcR, baSrcW)
			return fmt.Errorf("ab destination pipe: %w", err)
		}
		baDstR, baDstW, err = os.Pipe()
		if err != nil {
			closeAll(abSrcR, abSrcW, abDstR, abDstW, baSrcR, baSrcW)
			return fmt.Errorf("ba destination pipe: %w", err)
		}

		parentToClose = []*os.File{abSrcW, abDstR, baSrcW, baDstR}
		abRelayDoneCh := make(chan error, 1)
		baRelayDoneCh := make(chan error, 1)
		abRelayDone = abRelayDoneCh
		baRelayDone = baRelayDoneCh

		go func() {
			abRelayDoneCh <- runOnPinnedCPU(layout.abCPU, func() error {
				return relayUntilEOF(abSrcR, abDstW, mode.zeroCopy)
			})
		}()
		go func() {
			baRelayDoneCh <- runOnPinnedCPU(layout.baCPU, func() error {
				return relayUntilEOF(baSrcR, baDstW, mode.zeroCopy)
			})
		}()
	} else {
		parentToClose = []*os.File{abSrcR, abSrcW, baSrcR, baSrcW}
	}

	cmdA := exec.Command("python3", prodPath)
	cmdA.Stdin = baDstR
	cmdA.Stdout = abSrcW
	cmdA.Stderr = nil

	cmdB := exec.Command("python3", consPath)
	cmdB.Stdin = abDstR
	cmdB.Stdout = baSrcW
	cmdB.Stderr = nil

	start := time.Now()
	if err := cmdB.Start(); err != nil {
		closeRelayInputs(parentToClose...)
		return fmt.Errorf("start consumer failed: %w", err)
	}
	if err := setProcessCPU(cmdB.Process.Pid, layout.bCPU); err != nil {
		_ = cmdB.Process.Kill()
		return fmt.Errorf("pin consumer: %w", err)
	}
	if err := cmdA.Start(); err != nil {
		_ = cmdB.Process.Kill()
		return fmt.Errorf("start producer failed: %w", err)
	}
	if err := setProcessCPU(cmdA.Process.Pid, layout.aCPU); err != nil {
		_ = cmdA.Process.Kill()
		_ = cmdB.Process.Kill()
		return fmt.Errorf("pin producer: %w", err)
	}

	closeRelayInputs(parentToClose...)

	errA := cmdA.Wait()
	duration := time.Since(start)

	_ = cmdB.Process.Signal(os.Kill)
	_ = cmdB.Wait()

	if mode.proxy {
		if err := <-abRelayDone; err != nil {
			return fmt.Errorf("ab relay: %w", err)
		}
		if err := <-baRelayDone; err != nil {
			return fmt.Errorf("ba relay: %w", err)
		}
	}

	if duration.Milliseconds() < 500 {
		return fmt.Errorf("suspicious runtime: %v (too fast, likely crashed)", duration)
	}
	if errA != nil {
		return fmt.Errorf("producer crashed: %w", errA)
	}

	fmt.Printf("[%s/%s][ID %d] OK | WallTime: %v | User: %v | Sys: %v\n",
		mode.name, layout.name, id, duration, cmdA.ProcessState.UserTime(), cmdA.ProcessState.SystemTime())
	return nil
}

func relayUntilEOF(src *os.File, dst *os.File, zeroCopy bool) error {
	if zeroCopy {
		return spliceUntilEOF(src, dst)
	}
	return copyUntilEOF(src, dst)
}

func copyUntilEOF(src *os.File, dst *os.File) error {
	defer dst.Close()
	defer src.Close()

	_, err := io.Copy(dst, src)
	if err != nil {
		_, _ = io.Copy(io.Discard, src)
		return err
	}
	_, _ = io.Copy(io.Discard, src)
	return nil
}

func spliceUntilEOF(src *os.File, dst *os.File) error {
	defer dst.Close()
	defer src.Close()

	srcFd := int(src.Fd())
	dstFd := int(dst.Fd())
	discardFd := int(devNullFd())
	dstBroken := false

	for {
		targetFd := dstFd
		if dstBroken {
			targetFd = discardFd
		}
		n, err := unix.Splice(srcFd, nil, targetFd, nil, 64*1024, unix.SPLICE_F_MOVE)
		if err != nil {
			if !dstBroken {
				dstBroken = true
				continue
			}
			return err
		}
		if n == 0 {
			return nil
		}
	}
}

func runOnPinnedCPU(cpu int, fn func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var old unix.CPUSet
	if err := unix.SchedGetaffinity(0, &old); err != nil {
		return err
	}
	var next unix.CPUSet
	next.Set(cpu)
	if err := unix.SchedSetaffinity(0, &next); err != nil {
		return err
	}
	defer unix.SchedSetaffinity(0, &old)

	return fn()
}

func setProcessCPU(pid int, cpu int) error {
	var cpuset unix.CPUSet
	cpuset.Set(cpu)
	return unix.SchedSetaffinity(pid, &cpuset)
}

var (
	devNullOnce sync.Once
	devNullFile *os.File
)

func devNullFd() uintptr {
	devNullOnce.Do(func() {
		var err error
		devNullFile, err = os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err != nil {
			panic(err)
		}
	})
	return devNullFile.Fd()
}

func closeRelayInputs(files ...*os.File) {
	for _, f := range files {
		if f != nil {
			_ = f.Close()
		}
	}
}

func closeAll(files ...*os.File) {
	closeRelayInputs(files...)
}

func filterModes(name string) []relayMode {
	if name == "all" {
		return modes
	}
	for _, mode := range modes {
		if mode.name == name {
			return []relayMode{mode}
		}
	}
	panic("unknown mode: " + name)
}

func filterPlacements(name string) []placement {
	if name == "all" {
		return placements
	}
	for _, p := range placements {
		if p.name == name {
			return []placement{p}
		}
	}
	panic("unknown layout: " + name)
}

func modeNames(ms []relayMode) []string {
	ret := make([]string, 0, len(ms))
	for _, m := range ms {
		ret = append(ret, m.name)
	}
	return ret
}

func placementNames(ps []placement) []string {
	ret := make([]string, 0, len(ps))
	for _, p := range ps {
		ret = append(ret, p.name)
	}
	return ret
}
