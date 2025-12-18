package envexec

import (
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

var (
	devNull     *os.File
	devNullOnce sync.Once
)

// getDevNull returns a thread-safe, globally shared handle to /dev/null.
func getDevNull() *os.File {
	devNullOnce.Do(func() {
		var err error
		devNull, err = os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err != nil {
			// If we can't open /dev/null on a Linux system, something is
			// fundamentally broken with the environment.
			panic("critical: failed to open " + os.DevNull + ": " + err.Error())
		}
	})
	return devNull
}

func pipeProxyZeroCopy(p Pipe, out1 *os.File, in2 *os.File, buffer *os.File) *pipeCollector {
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer in2.Close()
		defer out1.Close()

		name := p.Name
		srcFd := int(out1.Fd())
		dstFd := int(in2.Fd())
		sideFd := int(buffer.Fd())
		discardFd := int(getDevNull().Fd())

		limit := int(p.Limit)
		totalProcessed := 0
		dstBroken := false

		for {
			// Determine chunk size (Standard Linux pipe capacity is 64KB)
			chunk := 64 * 1024
			if name != "" && totalProcessed < limit {
				remaining := limit - totalProcessed
				if chunk > remaining {
					chunk = remaining
				}
				if !dstBroken {
					n, err := unix.Tee(srcFd, dstFd, chunk, 0)
					if err != nil {
						dstBroken = true
						// Don't break; fall through to handle this chunk via Drain
					} else if n > 0 {
						// Successfully teed to destination, now move to storage
						_, err = unix.Splice(srcFd, nil, sideFd, nil, int(n), unix.SPLICE_F_MOVE)
						if err != nil {
							name = "" // Storage failed, stop future tee attempts
						}
						totalProcessed += int(n)
						continue
					} else {
						break // n == 0 (EOF)
					}
				} else {
					n, err := unix.Splice(srcFd, nil, sideFd, nil, chunk, unix.SPLICE_F_MOVE)
					if n <= 0 || err != nil {
						name = "" // Side-buffer failed or EOF
						if n == 0 {
							break
						}
						continue
					}
					totalProcessed += int(n)
					continue
				}
			}
			targetFd := dstFd
			if dstBroken {
				targetFd = discardFd
			}
			n, err := unix.Splice(srcFd, nil, targetFd, nil, chunk, unix.SPLICE_F_MOVE)
			if err != nil {
				if !dstBroken {
					dstBroken = true
					continue // Retry current chunk with devNullFd
				}
				break // Even draining to /dev/null failed
			}
			if n == 0 {
				break // EOF
			}
		}
	}()

	return &pipeCollector{
		done:    done,
		buffer:  buffer,
		limit:   p.Limit,
		name:    p.Name,
		storage: true,
	}
}
