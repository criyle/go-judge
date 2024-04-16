//go:build !seccomp

package env

import "syscall"

func readSeccompConf(name string) ([]syscall.SockFilter, error) {
	_ = name
	return nil, nil
}
