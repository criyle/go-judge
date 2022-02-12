package main

import "C"

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/criyle/go-judge/cmd/executorserver/model"
	"github.com/criyle/go-judge/env"
	"github.com/criyle/go-judge/env/pool"
	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-judge/filestore"
	"github.com/criyle/go-judge/worker"
)

type initParameter struct {
	CInitPath    string `json:"cinitPath"`
	Parallelism  int    `json:"parallelism"`
	TmpFsParam   string `json:"tmpfsParam"`
	Dir          string `json:"dir"`
	NetShare     bool   `json:"netShare"`
	MountConf    string `json:"mountConf"`
	SrcPrefix    string `json:"srcPrefix"`
	CgroupPrefix string `json:"cgroupPrefix"`
	CPUSet       string `json:"cpuset"`
	CredStart    int    `json:"credStart"`
}

var (
	fs   filestore.FileStore
	work worker.Worker

	srcPrefix string
)

func newFilsStore(dir string) filestore.FileStore {
	if dir == "" {
		if runtime.GOOS == "linux" {
			dir = "/dev/shm"
		} else {
			dir = os.TempDir()
		}
		dir, _ = os.MkdirTemp(dir, "executorserver")
	}
	os.MkdirAll(dir, 0755)
	return filestore.NewFileLocalStore(dir)
}

// Init initialize the sandbox environment
//export Init
func Init(i *C.char) C.int {
	is := C.GoString(i)
	var ip initParameter
	if err := json.NewDecoder(bytes.NewBufferString(is)).Decode(&ip); err != nil {
		return -1
	}

	if ip.Parallelism == 0 {
		ip.Parallelism = 4
	}

	if ip.TmpFsParam == "" {
		ip.TmpFsParam = "size=16m,nr_inodes=4k"
	}

	if ip.MountConf == "" {
		ip.MountConf = "mount.yaml"
	}

	srcPrefix = ip.SrcPrefix

	fs = newFilsStore(ip.Dir)

	b, err := env.NewBuilder(env.Config{
		ContainerInitPath:  ip.CInitPath,
		MountConf:          ip.MountConf,
		TmpFsParam:         ip.TmpFsParam,
		NetShare:           ip.NetShare,
		CgroupPrefix:       ip.CgroupPrefix,
		Cpuset:             ip.CPUSet,
		ContainerCredStart: ip.CredStart,
		Logger:             nopLogger{},
	})
	if err != nil {
		log.Fatalln("create environment builder failed", err)
	}
	envPool := pool.NewPool(b)
	work = worker.New(worker.Config{
		FileStore:             fs,
		EnvironmentPool:       envPool,
		Parallelism:           ip.Parallelism,
		WorkDir:               ip.Dir,
		TimeLimitTickInterval: 100 * time.Millisecond,
	})
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
	r, err := model.ConvertRequest(&req, srcPrefix)
	if err != nil {
		return nil
	}
	rtCh, _ := work.Submit(context.TODO(), r)
	rt := <-rtCh
	ret, err := model.ConvertResponse(rt, true)
	if err != nil {
		return nil
	}
	defer ret.Close()
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

	var fa fileAdd
	if err := json.NewDecoder(bytes.NewBufferString(es)).Decode(&fa); err != nil {
		return nil
	}

	f, err := fs.New()
	if err != nil {
		return nil
	}
	defer f.Close()

	if _, err := f.Write([]byte(fa.Content)); err != nil {
		return nil
	}
	id, err := fs.Add(fa.Name, f.Name())
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
	_, file := fs.Get(f.ID)
	if file == nil {
		return nil
	}
	r, err := envexec.FileToReader(file)
	if err != nil {
		return nil
	}
	if f, ok := r.(*os.File); ok {
		defer f.Close()
	}

	c, err := io.ReadAll(r)
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

type nopLogger struct {
}

func (nopLogger) Debug(args ...interface{}) {
}

func (nopLogger) Info(args ...interface{}) {
}

func (nopLogger) Warn(args ...interface{}) {
}

func (nopLogger) Error(args ...interface{}) {
}
