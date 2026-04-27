package envexec

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
)

// Standard Benchmark (The Baseline)
func BenchmarkDirectPipe(b *testing.B) {
	blockSizes := []int{1, 4, 16, 64, 256, 1024, 2048}

	for _, bs := range blockSizes {
		b.Run(fmt.Sprintf("BS-%dKB", bs), func(b *testing.B) {
			// Create a standard OS pipe without any proxy logic
			out, in, err := os.Pipe()
			if err != nil {
				b.Fatal(err)
			}

			dataChunk := make([]byte, bs*1024)
			b.SetBytes(int64(len(dataChunk)))

			go func() {
				io.Copy(io.Discard, out)
				out.Close()
			}()

			b.ResetTimer()
			for b.Loop() {
				_, err := in.Write(dataChunk)
				if err != nil {
					break
				}
			}
			in.Close()
		})
	}
}

func BenchmarkPipeProxy(b *testing.B) {
	blockSizes := []int{1, 4, 16, 64, 256, 1024, 2048}

	modes := []struct {
		name     string
		proxy    bool
		zeroCopy bool
	}{
		{"Baseline", false, false}, // Direct OS Pipe
		{"StdProxy", true, false},  // io.TeeReader (User-space)
		{"ZeroCopy", true, true},   // Splice/Tee (Kernel-space)
	}

	for _, mode := range modes {
		for _, bs := range blockSizes {
			b.Run(fmt.Sprintf("%s/BS-%dKB", mode.name, bs), func(b *testing.B) {
				p := Pipe{
					Proxy:           mode.proxy,
					DisableZeroCopy: !mode.zeroCopy,
					Name:            "bench-test",
					Limit:           1024 * 1024 * 10, // 10MB limit for the tee buffer
				}

				// Define our store file (the side-channel)
				newStore := func() (*os.File, error) {
					return os.OpenFile(os.DevNull, os.O_WRONLY, 0)
				}

				// Setup the pipe architecture
				out, in, pc, err := pipe(p, newStore)
				if err != nil {
					b.Fatal(err)
				}

				dataChunk := make([]byte, bs*1024)
				b.SetBytes(int64(len(dataChunk)))
				b.ResetTimer()

				// Start a consumer (The Sink)
				go func() {
					io.Copy(io.Discard, out)
				}()

				// Run the benchmark (The Source)
				for b.Loop() {
					_, err := in.Write(dataChunk)
					if err != nil {
						b.Log("write error:", err)
						break
					}
				}

				in.Close()
				if pc != nil && pc.done != nil {
					<-pc.done
				}
			})
		}
	}
}

func BenchmarkEmpiricalProxy(b *testing.B) {
	blockSizes := []int{1, 4, 16, 64, 256, 1024, 2048}

	modes := []struct {
		name     string
		proxy    bool
		zeroCopy bool
	}{
		{"Baseline", false, false}, // Direct OS Pipe
		{"StdProxy", true, false},  // io.TeeReader (User-space)
		{"ZeroCopy", true, true},   // Splice/Tee (Kernel-space)
	}

	for _, mode := range modes {
		for _, bs := range blockSizes {
			b.Run(fmt.Sprintf("%s/BS-%dK", mode.name, bs), func(b *testing.B) {
				p := Pipe{
					Proxy:           mode.proxy,
					DisableZeroCopy: !mode.zeroCopy,
					Name:            "bench",
					Limit:           1 << 60, // Set high so it doesn't stop early
				}

				newStore := func() (*os.File, error) {
					return os.OpenFile(os.DevNull, os.O_WRONLY, 0)
				}

				// Initialize your pipe architecture
				// out2 is the output for the next stage, in1 is the input for the previous
				out2, in1, pc, err := pipe(p, newStore)
				if err != nil {
					b.Fatal(err)
				}

				// Source: dd -> in1
				src := exec.Command("dd", "if=/dev/zero", fmt.Sprintf("bs=%dk", bs), fmt.Sprintf("count=%d", b.N), "status=none")
				src.Stdout = in1

				// Sink: out2 -> dd
				sink := exec.Command("dd", "of=/dev/null", fmt.Sprintf("bs=%dk", bs), "status=none")
				sink.Stdin = out2

				// Set bytes for throughput calculation
				b.SetBytes(int64(bs * 1024))
				b.ResetTimer()

				// Start the pipeline
				if err := src.Start(); err != nil {
					b.Fatal(err)
				}
				if err := sink.Start(); err != nil {
					b.Fatal(err)
				}

				// Wait for the source to finish writing and the sink to finish reading
				src.Wait()
				in1.Close() // Signal EOF to the proxy logic

				sink.Wait()
				out2.Close()

				if pc != nil && pc.done != nil {
					<-pc.done
				}
				b.StopTimer()
			})
		}
	}
}

func BenchmarkInteractiveProxy(b *testing.B) {
	messageSizes := []int{8, 64, 256, 1024}
	modes := []struct {
		name     string
		zeroCopy bool
	}{
		{"StdProxy", false},
		{"ZeroCopy", true},
	}

	for _, mode := range modes {
		for _, size := range messageSizes {
			b.Run(fmt.Sprintf("%s/Msg-%dB", mode.name, size), func(b *testing.B) {
				newStore := func() (*os.File, error) {
					return os.OpenFile(os.DevNull, os.O_WRONLY, 0)
				}

				pipeCfg := Pipe{
					Proxy:           true,
					DisableZeroCopy: !mode.zeroCopy,
				}

				abOut, abIn, abCollector, err := pipe(pipeCfg, newStore)
				if err != nil {
					b.Fatal(err)
				}
				baOut, baIn, baCollector, err := pipe(pipeCfg, newStore)
				if err != nil {
					b.Fatal(err)
				}
				defer func() {
					abIn.Close()
					abOut.Close()
					baIn.Close()
					baOut.Close()
					if abCollector != nil && abCollector.done != nil {
						<-abCollector.done
					}
					if baCollector != nil && baCollector.done != nil {
						<-baCollector.done
					}
				}()

				payload := make([]byte, size)
				reply := make([]byte, size)

				echoDone := make(chan error, 1)
				go func() {
					buf := make([]byte, size)
					for {
						if _, err := io.ReadFull(abOut, buf); err != nil {
							if err == io.EOF || err == io.ErrUnexpectedEOF {
								echoDone <- nil
							} else {
								echoDone <- err
							}
							return
						}
						if _, err := baIn.Write(buf); err != nil {
							echoDone <- err
							return
						}
					}
				}()

				b.SetBytes(int64(size))
				b.ResetTimer()
				for b.Loop() {
					if _, err := abIn.Write(payload); err != nil {
						b.Fatal(err)
					}
					if _, err := io.ReadFull(baOut, reply); err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()

				abIn.Close()
				baIn.Close()
				if err := <-echoDone; err != nil {
					b.Fatal(err)
				}
				abOut.Close()
			})
		}
	}
}
