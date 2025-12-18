package envexec

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

// Verification Test: Ensures data integrity across all branches
func TestProxyDataIntegrity(t *testing.T) {
	// 1. Setup parameters
	dataSize := 10 * 1024 * 1024 // 10MB
	limit := 5 * 1024 * 1024     // Limit tee to first 5MB
	blockSize := "64k"

	// Create random source data
	srcData := make([]byte, dataSize)
	rand.Read(srcData)
	srcFile, _ := os.CreateTemp("", "src_data")
	defer os.Remove(srcFile.Name())
	srcFile.Write(srcData)
	srcFile.Close()

	// Create temp files for outputs
	mainOutFile, _ := os.CreateTemp("", "main_out")
	sideBufferFile, _ := os.CreateTemp("", "side_buffer")
	defer os.Remove(mainOutFile.Name())
	defer os.Remove(sideBufferFile.Name())

	for _, zeroCopy := range []bool{false, true} {
		t.Run(fmt.Sprintf("ZeroCopy-%v", zeroCopy), func(t *testing.T) {
			// 2. Initialize the Pipe Architecture
			p := Pipe{
				Proxy:           true,
				DisableZeroCopy: !zeroCopy,
				Name:            "integrity-test",
				Limit:           Size(limit),
			}

			newStore := func() (*os.File, error) {
				return os.OpenFile(sideBufferFile.Name(), os.O_WRONLY|os.O_TRUNC, 0666)
			}

			outPipe, inPipe, pc, err := pipe(p, newStore)
			if err != nil {
				t.Fatalf("Failed to create pipe: %v", err)
			}

			// 3. Setup External Processes (Simulating the environment)
			// dd source -> proxy input
			srcCmd := exec.Command("dd", fmt.Sprintf("if=%s", srcFile.Name()), "bs="+blockSize, "status=none")
			srcCmd.Stdout = inPipe

			// proxy output -> dd sink
			sinkCmd := exec.Command("dd", fmt.Sprintf("of=%s", mainOutFile.Name()), "bs="+blockSize, "status=none")
			sinkCmd.Stdin = outPipe

			// 4. Execute
			if err := srcCmd.Start(); err != nil {
				t.Fatal(err)
			}
			if err := sinkCmd.Start(); err != nil {
				t.Fatal(err)
			}

			// Wait for completion
			srcCmd.Wait()
			inPipe.Close()
			sinkCmd.Wait()
			outPipe.Close()

			if pc != nil && pc.done != nil {
				<-pc.done
			}

			// 5. Verification Logic
			// Verify Main Output (Should be 100% identical to source)
			gotMain, _ := os.ReadFile(mainOutFile.Name())
			if !bytes.Equal(srcData, gotMain) {
				t.Errorf("Main output data corruption! Sizes: src=%d, got=%d", len(srcData), len(gotMain))
			} else {
				t.Log("✅ Main output integrity verified.")
			}

			// Verify Side Buffer (Should be identical to the first 'limit' bytes of source)
			gotSide, _ := os.ReadFile(sideBufferFile.Name())
			expectedSide := srcData[:limit]
			if !bytes.Equal(expectedSide, gotSide) {
				t.Errorf("Side buffer (tee) data corruption! Expected size %d, got %d", limit, len(gotSide))
			} else {
				t.Log("✅ Side buffer (tee) integrity verified.")
			}
		})
	}
}

func TestProxyDrainOnConsumerExit(t *testing.T) {
	// 1. Setup Parameters
	totalDataSize := 20 * 1024 * 1024 // 20MB
	limit := 10 * 1024 * 1024         // Record first 10MB
	earlyExitSize := 2 * 1024 * 1024  // Consumer exits after only 2MB
	blockSize := "64k"

	// Create random source data
	srcData := make([]byte, totalDataSize)
	rand.Read(srcData)
	srcFile, _ := os.CreateTemp("", "drain_src")
	defer os.Remove(srcFile.Name())
	srcFile.Write(srcData)
	srcFile.Close()

	sideBufferFile, _ := os.CreateTemp("", "drain_side")
	defer os.Remove(sideBufferFile.Name())

	for _, zeroCopy := range []bool{false, true} {
		t.Run(fmt.Sprintf("ZeroCopy-%v", zeroCopy), func(t *testing.T) {
			// 2. Initialize the Pipe Architecture
			p := Pipe{
				Proxy:           true,
				DisableZeroCopy: !zeroCopy,
				Name:            "drain-test",
				Limit:           Size(limit),
			}

			newStore := func() (*os.File, error) {
				return os.OpenFile(sideBufferFile.Name(), os.O_WRONLY|os.O_TRUNC, 0666)
			}

			outPipe, inPipe, pc, err := pipe(p, newStore)
			if err != nil {
				t.Fatalf("Failed to create pipe: %v", err)
			}

			// 3. Setup Processes
			// Source: Sends the full 20MB
			srcCmd := exec.Command("dd", fmt.Sprintf("if=%s", srcFile.Name()), "bs="+blockSize, "status=none")
			srcCmd.Stdout = inPipe

			// Sink: Only reads 2MB and then EXITS
			sinkCmd := exec.Command("dd", "of=/dev/null", "bs="+blockSize, fmt.Sprintf("count=%d", earlyExitSize/(64*1024)), "status=none")
			sinkCmd.Stdin = outPipe

			// 4. Execution
			if err := srcCmd.Start(); err != nil {
				t.Fatal(err)
			}
			if err := sinkCmd.Start(); err != nil {
				t.Fatal(err)
			}

			// The sink will finish very quickly
			sinkCmd.Wait()
			outPipe.Close() // Close the read end since sink is gone

			// CRITICAL: The srcCmd must still be able to finish even though sinkCmd is dead.
			// If the proxy doesn't "drain," srcCmd will hang here forever.
			srcDone := make(chan error, 1)
			go func() {
				srcDone <- srcCmd.Wait()
				inPipe.Close()
			}()

			select {
			case err := <-srcDone:
				if err != nil {
					t.Logf("Source exited with error (expected SIGPIPE if not drained): %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("TIMEOUT: Source is blocked! Proxy failed to drain data after consumer exit.")
			}

			if pc != nil && pc.done != nil {
				<-pc.done
			}

			// 5. Verification
			// The side buffer should still have captured exactly 'limit' bytes
			gotSide, _ := os.ReadFile(sideBufferFile.Name())
			if len(gotSide) != limit {
				t.Errorf("Side buffer size mismatch. Expected %d, got %d. Drain logic might be skipping tee steps.", limit, len(gotSide))
			} else {
				t.Log("✅ Side buffer integrity verified despite consumer exit.")
			}
		})
	}
}
