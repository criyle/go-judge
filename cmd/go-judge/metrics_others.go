//go:build !linux

package main

import "github.com/criyle/go-judge/cmd/go-judge/config"

func initCgroupMetrics(conf *config.Config, param map[string]any) {
}
