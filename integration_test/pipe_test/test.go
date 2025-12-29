//go:build integration

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// --- Configuration ---
const iterCount = 100000 // Enough work to take ~2-4 seconds

// The Producer (Program A): Drives the interaction
const producerScript = `
import sys
import time
import random

# Force unbuffered I/O behavior
sys.stdout.reconfigure(line_buffering=True)

# 1. Handshake (ensure B is alive)
sys.stdout.write("PING\n")
sys.stdout.flush()
reply = sys.stdin.readline()
if not reply:
    sys.stderr.write("Producer: Peer disconnected immediately\n")
    sys.exit(1)

# 2. The Work Loop
start = time.time()
for i in range(%d):
    # Send
    sys.stdout.write(f"{i}\n")
    # Read
    _ = sys.stdin.readline()

duration = time.time() - start
sys.stderr.write(f"Producer: Done {i+1} iters in {duration:.4f}s\n")
`

// The Consumer (Program B): Echoes everything back
const consumerScript = `
import sys

sys.stdout.reconfigure(line_buffering=True)

while True:
    line = sys.stdin.readline()
    if not line:
        break
    sys.stdout.write(line)
`

func main() {
	parallelism := flag.Int("p", 1, "Number of parallel executions")
	totalRuns := flag.Int("n", 1, "Total number of tests to run")
	flag.Parse()

	// 1. Setup Scripts
	tmpDir, err := os.MkdirTemp("", "pipe_repro")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	prodPath := filepath.Join(tmpDir, "producer.py")
	consPath := filepath.Join(tmpDir, "consumer.py")

	// Inject the iteration count into the script
	fullProdScript := fmt.Sprintf(producerScript, iterCount)

	os.WriteFile(prodPath, []byte(fullProdScript), 0755)
	os.WriteFile(consPath, []byte(consumerScript), 0755)

	fmt.Printf("--- Starting Test ---\n")
	fmt.Printf("Scripts prepared in: %s\n", tmpDir)
	fmt.Printf("Workload: %d iterations per process\n", iterCount)
	fmt.Printf("Parallelism: %d\n", *parallelism)
	fmt.Printf("Total Runs:  %d\n\n", *totalRuns)

	// 2. Execution Loop
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, *parallelism)

	var successCount int64
	var failCount int64

	globalStart := time.Now()

	for i := 0; i < *totalRuns; i++ {
		wg.Add(1)
		semaphore <- struct{}{} // Acquire token

		go func(id int) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release token

			// Run the single instance
			err := runInstance(id, prodPath, consPath)
			if err != nil {
				fmt.Printf("[ID %d] FAILED: %v\n", id, err)
				atomic.AddInt64(&failCount, 1)
			} else {
				atomic.AddInt64(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()
	fmt.Printf("\n--- Summary ---\n")
	fmt.Printf("Total Time: %v\n", time.Since(globalStart))
	fmt.Printf("Success: %d, Failed: %d\n", successCount, failCount)
}

func runInstance(id int, prodPath, consPath string) error {
	// A. Create Pipes
	// A_Stdout -> B_Stdin
	r1, w1, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("pipe1 creation: %w", err)
	}

	// B_Stdout -> A_Stdin
	r2, w2, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("pipe2 creation: %w", err)
	}

	// B. Setup Producer (A)
	cmdA := exec.Command("python3", prodPath)
	cmdA.Stdin = r2   // Read from B
	cmdA.Stdout = w1  // Write to B
	cmdA.Stderr = nil // discard stderr unless debugging

	// C. Setup Consumer (B)
	cmdB := exec.Command("python3", consPath)
	cmdB.Stdin = r1  // Read from A
	cmdB.Stdout = w2 // Write to A
	cmdB.Stderr = nil

	// D. Start Processes
	start := time.Now()

	// IMPORTANT: We must close the unused pipe ends in the parent
	// or we will leak FDs and never get EOF.
	if err := cmdB.Start(); err != nil {
		return fmt.Errorf("start Consumer failed: %w", err)
	}
	if err := cmdA.Start(); err != nil {
		// If A fails, kill B
		cmdB.Process.Kill()
		return fmt.Errorf("start Producer failed: %w", err)
	}

	// Close Parent's copy of the pipes so only the children hold them
	r1.Close()
	w1.Close()
	r2.Close()
	w2.Close()

	// E. Wait for Producer to Finish
	// We assume Producer drives the logic. When it exits, we are done.
	errA := cmdA.Wait()
	duration := time.Since(start)

	// Kill Consumer (it's an infinite loop)
	cmdB.Process.Signal(os.Kill)
	cmdB.Wait()

	// F. Analysis
	if duration.Milliseconds() < 500 {
		return fmt.Errorf("SUSPICIOUS RUNTIME: %v (Too fast! Likely crashed)", duration)
	}

	if errA != nil {
		return fmt.Errorf("producer crashed: %v", errA)
	}

	userTime := cmdA.ProcessState.UserTime()
	sysTime := cmdA.ProcessState.SystemTime()

	fmt.Printf("[ID %d] OK | WallTime: %v | User Time: %v | Sys Time: %v\n", id, duration, userTime, sysTime)
	return nil
}
