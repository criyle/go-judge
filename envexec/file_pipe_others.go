//go:build !linux

package envexec

import "os"

func pipeProxyZeroCopy(p Pipe, out1 *os.File, in2 *os.File, buffer *os.File) *pipeCollector {
	return pipeProxy(p, out1, in2, buffer)
}
