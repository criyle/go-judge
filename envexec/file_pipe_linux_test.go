package envexec

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"golang.org/x/sys/unix"
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

func BenchmarkProxyAffinity(b *testing.B) {
	if runtime.NumCPU() < 3 {
		b.Skip("requires at least 3 CPUs")
	}

	modes := []struct {
		name     string
		zeroCopy bool
	}{
		{"StdProxy", false},
		{"ZeroCopy", true},
	}
	messageSizes := []int{8, 64, 256, 1024}
	placements := []struct {
		name       string
		writerCPU  int
		splicerCPU int
		readerCPU  int
	}{
		{"AllSameCore", 0, 0, 0},
		{"WriterReaderSame_SplicerOther", 0, 1, 0},
		{"AllDifferentCore", 0, 1, 2},
	}

	for _, mode := range modes {
		for _, messageSize := range messageSizes {
			payload := make([]byte, messageSize)
			for _, placement := range placements {
				b.Run(fmt.Sprintf("%s/Msg-%dB/%s", mode.name, messageSize, placement.name), func(b *testing.B) {
					out1, in1, out2, in2, err := pipe2()
					if err != nil {
						b.Fatal(err)
					}
					defer out2.Close()

					splicerDone := make(chan error, 1)
					readerDone := make(chan error, 1)
					writerDone := make(chan error, 1)

					go func() {
						splicerDone <- runOnPinnedCPU(placement.splicerCPU, func() error {
							return relayUntilEOF(out1, in2, mode.zeroCopy)
						})
					}()

					go func() {
						readerDone <- runOnPinnedCPU(placement.readerCPU, func() error {
							buf := make([]byte, messageSize)
							for i := 0; i < b.N; i++ {
								if _, err := io.ReadFull(out2, buf); err != nil {
									return err
								}
							}
							return nil
						})
					}()

					b.SetBytes(int64(messageSize))
					b.ResetTimer()

					go func() {
						writerDone <- runOnPinnedCPU(placement.writerCPU, func() error {
							defer in1.Close()
							for i := 0; i < b.N; i++ {
								if _, err := in1.Write(payload); err != nil {
									return err
								}
							}
							return nil
						})
					}()

					if err := <-writerDone; err != nil {
						b.Fatal(err)
					}
					if err := <-readerDone; err != nil {
						b.Fatal(err)
					}
					if err := <-splicerDone; err != nil {
						b.Fatal(err)
					}
					b.StopTimer()
				})
			}
		}
	}
}

func BenchmarkProxyBidirectionalAffinity(b *testing.B) {
	if runtime.NumCPU() < 4 {
		b.Skip("requires at least 4 CPUs")
	}

	modes := []struct {
		name     string
		zeroCopy bool
	}{
		{"StdProxy", false},
		{"ZeroCopy", true},
	}
	messageSizes := []int{8, 64, 256, 1024}
	placements := []struct {
		name  string
		aCPU  int
		bCPU  int
		abCPU int
		baCPU int
	}{
		{"AllSameCore", 0, 0, 0, 0},
		{"WriterReaderSame_SplicersOther", 0, 0, 1, 1},
		{"EachRoleSplit", 0, 1, 2, 3},
	}

	for _, mode := range modes {
		for _, messageSize := range messageSizes {
			payload := make([]byte, messageSize)
			reply := make([]byte, messageSize)
			for _, placement := range placements {
				b.Run(fmt.Sprintf("%s/Msg-%dB/%s", mode.name, messageSize, placement.name), func(b *testing.B) {
					abOut1, abIn1, abOut2, abIn2, err := pipe2()
					if err != nil {
						b.Fatal(err)
					}
					baOut1, baIn1, baOut2, baIn2, err := pipe2()
					if err != nil {
						b.Fatal(err)
					}
					defer abOut2.Close()
					defer baOut2.Close()

					abSplicerDone := make(chan error, 1)
					baSplicerDone := make(chan error, 1)
					aDone := make(chan error, 1)
					bDone := make(chan error, 1)

					go func() {
						abSplicerDone <- runOnPinnedCPU(placement.abCPU, func() error {
							return relayUntilEOF(abOut1, abIn2, mode.zeroCopy)
						})
					}()
					go func() {
						baSplicerDone <- runOnPinnedCPU(placement.baCPU, func() error {
							return relayUntilEOF(baOut1, baIn2, mode.zeroCopy)
						})
					}()

					go func() {
						bDone <- runOnPinnedCPU(placement.bCPU, func() error {
							buf := make([]byte, messageSize)
							for {
								if _, err := io.ReadFull(abOut2, buf); err != nil {
									if err == io.EOF || err == io.ErrUnexpectedEOF {
										return nil
									}
									return err
								}
								if _, err := baIn1.Write(buf); err != nil {
									return err
								}
							}
						})
					}()

					b.SetBytes(int64(messageSize))
					b.ResetTimer()
					go func() {
						aDone <- runOnPinnedCPU(placement.aCPU, func() error {
							defer abIn1.Close()
							for i := 0; i < b.N; i++ {
								if _, err := abIn1.Write(payload); err != nil {
									return err
								}
								if _, err := io.ReadFull(baOut2, reply); err != nil {
									return err
								}
							}
							return nil
						})
					}()

					if err := <-aDone; err != nil {
						b.Fatal(err)
					}
					b.StopTimer()

					baIn1.Close()
					if err := <-bDone; err != nil {
						b.Fatal(err)
					}
					if err := <-abSplicerDone; err != nil {
						b.Fatal(err)
					}
					if err := <-baSplicerDone; err != nil {
						b.Fatal(err)
					}
				})
			}
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
	discardFd := int(getDevNull().Fd())
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
