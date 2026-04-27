//go:build !linux

package envexec

func runWithCPUAffinity(_ string, fn func()) {
	fn()
}
