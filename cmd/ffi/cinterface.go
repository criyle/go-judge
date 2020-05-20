package main

import "C"

import (
	"bytes"
	"encoding/json"
	"log"
	"os"

	"github.com/criyle/go-judge/cmd/executorserver/model"
	"github.com/criyle/go-judge/env"
	"github.com/criyle/go-judge/filestore"
	"github.com/criyle/go-judge/pkg/pool"
	"github.com/criyle/go-judge/worker"
)

type initParameter struct {
	CInitPath  string `json:"cinitPath`
	Parallism  int    `json:"parallism"`
	TmpFsParam string `json:"tmpfsParam"`
	Dir        string `json:"dir"`
	NetShare   bool   `json:"netShare"`
	MountConf  string `json:"mountConf"`
}

var (
	fs   filestore.FileStore
	work *worker.Worker
)

func newFilsStore(dir string) filestore.FileStore {
	var fs filestore.FileStore
	if dir == "" {
		fs = filestore.NewFileMemoryStore()
	} else {
		os.MkdirAll(dir, 0755)
		fs = filestore.NewFileLocalStore(dir)
	}
	return fs
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

	if ip.TmpFsParam == "" {
		ip.TmpFsParam = "size=16m,nr_inodes=4k"
	}

	if ip.MountConf == "" {
		ip.MountConf = "mount.yaml"
	}

	fs = newFilsStore(ip.Dir)

	printLog := func(v ...interface{}) {}
	b, err := env.NewBuilder(ip.CInitPath, ip.MountConf, ip.TmpFsParam, ip.NetShare, printLog)
	if err != nil {
		log.Fatalln("create environment builder failed", err)
	}
	envPool := pool.NewPool(b)
	work = worker.New(fs, envPool, ip.Parallism, ip.Dir)
	work.Start()

	return 0
}

// Exec runs command inside container runner
//export Exec
func Exec(e *C.char) *C.char {
	es := C.GoString(e)
	var req model.Request
	if err := json.NewDecoder(bytes.NewBufferString(es)).Decode(&req); err != nil {
		return nil
	}
	r, err := model.ConvertRequest(&req)
	if err != nil {
		return nil
	}
	rt := <-work.Submit(r)
	ret := model.ConvertResponse(rt)
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
