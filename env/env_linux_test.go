//go:build linux

package env

import "testing"

func TestKernelVersionAtLeast(t *testing.T) {
	tests := []struct {
		name                         string
		major, minor                 int
		requiredMajor, requiredMinor int
		want                         bool
	}{
		{name: "older major", major: 5, minor: 15, requiredMajor: 6, requiredMinor: 9, want: false},
		{name: "older minor", major: 6, minor: 8, requiredMajor: 6, requiredMinor: 9, want: false},
		{name: "exact", major: 6, minor: 9, requiredMajor: 6, requiredMinor: 9, want: true},
		{name: "newer minor", major: 6, minor: 10, requiredMajor: 6, requiredMinor: 9, want: true},
		{name: "newer major", major: 7, minor: 0, requiredMajor: 6, requiredMinor: 9, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := kernelVersionAtLeast(tt.major, tt.minor, tt.requiredMajor, tt.requiredMinor); got != tt.want {
				t.Fatalf("kernelVersionAtLeast(%d, %d, %d, %d) = %v, want %v",
					tt.major, tt.minor, tt.requiredMajor, tt.requiredMinor, got, tt.want)
			}
		})
	}
}
