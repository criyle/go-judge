// Package env provides a unified method to create environment for envexec.
//
// For linux, the env creates container & cgroup sandbox.
//
// For windows, the env creates low mandatory level sandbox.
//
// For macOS, the env creates sandbox_init sandbox.
package env
