package grpcexecutor

import "unsafe"

func byteArrayToString(buf []byte) string {
	return *(*string)(unsafe.Pointer(&buf))
}
