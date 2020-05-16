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

// FileList get the list of files in the file store
//export FileList
func FileList() *C.char {
	ids := fs.List()

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(ids); err != nil {
		return nil
	}
	return C.CString(buf.String())
}

// FileAdd adds file to the file store
//export FileAdd
func FileAdd(e *C.char) *C.char {
	type fileAdd struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	es := C.GoString(e)

	var f fileAdd
	if err := json.NewDecoder(bytes.NewBufferString(es)).Decode(&f); err != nil {
		return nil
	}

	id, err := fs.Add(f.Name, []byte(f.Content))
	if err != nil {
		return nil
	}
	return C.CString(id)
}

// FileGet gets file from file store by id
//export FileGet
func FileGet(e *C.char) *C.char {
	type fileGet struct {
		ID string `json:"id"`
	}
	es := C.GoString(e)

	var f fileGet
	if err := json.NewDecoder(bytes.NewBufferString(es)).Decode(&f); err != nil {
		return nil
	}
	file := fs.Get(f.ID)
	if file == nil {
		return nil
	}
	c, err := file.Content()
	if err != nil {
		return nil
	}
	return C.CString(string(c))
}

// FileDelete deletes file from file store by id
//export FileDelete
func FileDelete(e *C.char) *C.char {
	type fileDelete struct {
		ID string `json:"id"`
	}
	es := C.GoString(e)

	var f fileDelete
	if err := json.NewDecoder(bytes.NewBufferString(es)).Decode(&f); err != nil {
		return nil
	}
	ok := fs.Remove(f.ID)
	if !ok {
		return nil
	}
	return C.CString("")
}
