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
	"strings"
	"time"
	"unsafe"

	"github.com/criyle/go-judge/cmd/go-judge/model"
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

	srcPrefix []string
)

func newFilsStore(dir string) (filestore.FileStore, error) {
	if dir == "" {
		if runtime.GOOS == "linux" {
			dir = "/dev/shm"
		} else {
			dir = os.TempDir()
		}
		dir, _ = os.MkdirTemp(dir, "go-judge")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return filestore.NewFileLocalStore(dir), nil
}

// Init initialize the sandbox environment
//
//export Init
func Init(i *C.char) C.int {
	is := C.GoString(i)
	var ip initParameter
	if err := json.NewDecoder(bytes.NewBufferString(is)).Decode(&ip); err != nil {
		return -1
	}

	if ip.Parallelism <= 0 {
		ip.Parallelism = 4
	}

	if ip.TmpFsParam == "" {
		ip.TmpFsParam = "size=16m,nr_inodes=4k"
	}

	if ip.MountConf == "" {
		ip.MountConf = "mount.yaml"
	}

	srcPrefix = strings.Split(ip.SrcPrefix, ",")

	var err error
	fs, err = newFilsStore(ip.Dir)
	if err != nil {
		log.Fatalln("file store create failed", err)
	}

	b, _, err := env.NewBuilder(env.Config{
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
//
// Remember to free the return char pointer value
//
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

// FileList get the list of files in the file store.
//
// Remember to free the 2-d char array `ids` and `names`
//
//export FileList
func FileList(ids ***C.char, names ***C.char) C.size_t {
	res := fs.List()
	idsWrap := C.malloc(C.size_t(len(res)) * C.size_t(unsafe.Sizeof(uintptr(0))))
	namesWrap := C.malloc(C.size_t(len(res)) * C.size_t(unsafe.Sizeof(uintptr(0))))
	pIDsWrap := (*[1<<30 - 1]*C.char)(idsWrap)
	pNamesWrap := (*[1<<30 - 1]*C.char)(namesWrap)
	idx := 0
	for id, name := range res {
		pIDsWrap[idx] = C.CString(id)
		pNamesWrap[idx] = C.CString(name)
		idx++
	}
	*ids = (**C.char)(idsWrap)
	*names = (**C.char)(namesWrap)
	return C.size_t(len(res))
}

// FileAdd adds file to the file store
//
// Remember to free the return char pointer value
//
//export FileAdd
func FileAdd(content *C.char, contentLen C.int, name *C.char) *C.char {
	sContent := C.GoBytes(unsafe.Pointer(content), contentLen)

	f, err := fs.New()
	if err != nil {
		return nil
	}
	defer f.Close()

	if _, err := f.Write(sContent); err != nil {
		return nil
	}
	id, err := fs.Add(C.GoString(name), f.Name())
	if err != nil {
		return nil
	}
	return C.CString(id)
}

// FileGet gets file from file store by id.
// If the return value is a positive number or zero, the value represents the length of the file.
// Otherwise, if the return value is negative, the following error occurred:
//
// - `-1`: The file does not exist.
// - `-2`: go-judge internal error.
//
// Remember to free `out`.
//
//export FileGet
func FileGet(e *C.char, out **C.char) C.int {
	es := C.GoString(e)
	_, file := fs.Get(es)
	if file == nil {
		return -1
	}
	r, err := envexec.FileToReader(file)
	if err != nil {
		return -2
	}
	defer r.Close()

	c, err := io.ReadAll(r)
	if err != nil {
		return -2
	}
	*out = (*C.char)(C.CBytes(c))
	return (C.int)(len(c))
}

// FileDelete deletes file from file store by id, returns 0 if failed.
//
//export FileDelete
func FileDelete(e *C.char) C.int {
	es := C.GoString(e)
	ok := fs.Remove(es)
	if !ok {
		return 0
	}
	return 1
}

type nopLogger struct{}

func (nopLogger) Debug(args ...interface{}) {
}

func (nopLogger) Info(args ...interface{}) {
}

func (nopLogger) Warn(args ...interface{}) {
}

func (nopLogger) Error(args ...interface{}) {
}
