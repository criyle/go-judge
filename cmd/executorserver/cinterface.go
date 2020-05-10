package main

import "C"

import (
	"bytes"
	"encoding/json"
	"os"
)

type initParameter struct {
	CInitPath  string `json:"cinitPath`
	Parallism  int    `json:"parallism"`
	TmpFsParam string `json:"tmpfsParam"`
	Dir        string `json:"dir"`
	NetShare   bool   `json:"netShare"`
	MountConf  string `json:"mountConf"`
}

// Init initialize the sandbox environment
//export Init
func Init(i *C.char) C.int {
	is := C.GoString(i)
	var ip initParameter
	if err := json.NewDecoder(bytes.NewBufferString(is)).Decode(&ip); err != nil {
		return -1
	}

	if ip.Parallism == 0 {
		ip.Parallism = 4
	}
	parallism = &ip.Parallism

	if ip.TmpFsParam == "" {
		ip.TmpFsParam = "size=16m,nr_inodes=4k"
	}
	tmpFsParam = &ip.TmpFsParam

	if ip.MountConf == "" {
		ip.MountConf = "mount.yaml"
	}

	if ip.Dir == "" {
		fs = newFileMemoryStore()
	} else {
		os.MkdirAll(ip.Dir, 0755)
		fs = newFileLocalStore(ip.Dir)
	}
	cinitPath = &ip.CInitPath

	printLog = func(v ...interface{}) {}
	initEnvPool()

	startWorkers()
	return 0
}

// Exec runs command inside container runner
//export Exec
func Exec(e *C.char) *C.char {
	es := C.GoString(e)
	var req request
	if err := json.NewDecoder(bytes.NewBufferString(es)).Decode(&req); err != nil {
		return nil
	}
	ret := <-submitRequest(&req)
	if ret.Error != nil {
		ret.ErrorMsg = ret.Error.Error()
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(ret); err != nil {
		return nil
	}
	return C.CString(buf.String())
}
