//go:build !seccomp

package env

import "syscall"

func readSeccompConf(name string) ([]syscall.SockFilter, error) {
	return nil, nil
}
