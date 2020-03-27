// Package envexec provides utility function to run program in restricted environments
// through container and cgroup.
//
// Cmd
//
// Cmd defines single program to run, including copyin files before exec, run the program and copy
// out files after exec
//
// Single
//
// Single defines single Cmd with Environment and Cgroup Pool
//
// Group
//
// Group defines multiple Cmd with Environment and Cgroup Pool, together with Pipe mapping between
// different Cmd
package envexec
