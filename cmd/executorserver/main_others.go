// +build !windows,!linux,!darwin

package main

import "github.com/criyle/go-judge/pkg/envexec"

func newEnvPool() envexec.EnvironmentPool {
	return nil
}
