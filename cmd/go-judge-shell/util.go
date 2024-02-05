package main

import "unsafe"

func strToBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

func byteArrayToString(buf []byte) string {
	return *(*string)(unsafe.Pointer(&buf))
}
