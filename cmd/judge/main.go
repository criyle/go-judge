package main

import (
	"io/ioutil"
	"log"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/criyle/go-judge/client/syzojclient"
	"github.com/criyle/go-judge/file"
	"github.com/criyle/go-judge/file/memfile"
	"github.com/criyle/go-judge/judger"
	"github.com/criyle/go-judge/language"
	"github.com/criyle/go-judge/runner"
	"github.com/criyle/go-judge/taskqueue/channel"
	"github.com/criyle/go-judge/types"
	"github.com/criyle/go-sandbox/container"
	"github.com/criyle/go-sandbox/pkg/cgroup"
	"github.com/criyle/go-sandbox/pkg/mount"
	stypes "github.com/criyle/go-sandbox/types"
)

func init() {
	container.Init()
}

func main() {
	log.Printf("GOARCH=%s GOOS=%s", runtime.GOARCH, runtime.GOOS)

	var url = os.Getenv("WEB_URL")
	c, errCh, err := syzojclient.NewClient(url, "123")
	if err != nil {
		panic(err)
	}
	log.Printf("Connected to %s", url)

	var wg sync.WaitGroup

	done := make(chan struct{})
	root, err := ioutil.TempDir("", "dm")
	if err != nil {
		panic(err)
	}
	q := channel.New()
	m, err := mount.NewBuilder().
		// basic exec and lib
		WithBind("/bin", "bin", true).
		WithBind("/lib", "lib", true).
		WithBind("/lib64", "lib64", true).
		WithBind("/usr", "usr", true).
		// java wants /proc/self/exe as it need relative path for lib
		// however, /proc gives interface like /proc/1/fd/3 ..
		// it is fine since open that file will be a EPERM
		// changing the fs uid and gid would be a good idea
		WithProc().
		// some compiler have multiple version
		WithBind("/etc/alternatives", "etc/alternatives", true).
		// fpc wants /etc/fpc.cfg
		WithBind("/etc/fpc.cfg", "etc/fpc.cfg", true).
		// go wants /dev/null
		WithBind("/dev/null", "dev/null", false).
		// work dir
		WithTmpfs("w", "size=8m,nr_inodes=4k").
		// tmp dir
		WithTmpfs("tmp", "size=8m,nr_inodes=4k").
		// finished
		Build(true)

	if err != nil {
		panic(err)
	}
	b := &container.Builder{
		Root:          root,
		Mounts:        m,
		CredGenerator: newCredGen(),
		Stderr:        true,
	}
	cgb, err := cgroup.NewBuilder("go-judger").WithCPUAcct().WithMemory().WithPids().FilterByEnv()
	if err != nil {
		panic(err)
	}
	log.Printf("Initialized cgroup: %v", cgb)
	r := &runner.Runner{
		Builder:       b,
		Queue:         q,
		CgroupBuilder: cgb,
		Language:      &dumbLang{},
	}
	const parallism = 4
	for i := 0; i < parallism; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Loop(done)
		}()
	}

	j := &judger.Judger{
		Client:  c,
		Sender:  q,
		Builder: &dumbBuilder{},
	}
	go j.Loop(done)

	go func() {
		panic(<-errCh)
	}()
	<-c.Done
}

type credGen struct {
	cur uint32
}

func newCredGen() *credGen {
	return &credGen{cur: 10000}
}

func (c *credGen) Get() syscall.Credential {
	n := atomic.AddUint32(&c.cur, 1)
	return syscall.Credential{
		Uid: n,
		Gid: n,
	}
}

type dumbBuilder struct {
}

func (b *dumbBuilder) Build([]file.File) (types.ProblemConfig, error) {
	const n = 100

	c := make([]types.Case, 0, n)
	for i := 0; i < n; i++ {
		inputContent := strconv.Itoa(i) + " " + strconv.Itoa(i)
		outputContent := strconv.Itoa(i + i)
		c = append(c, types.Case{
			Input:  memfile.New("input", []byte(inputContent)),
			Answer: memfile.New("output", []byte(outputContent)),
		})
	}

	return types.ProblemConfig{
		Type: "standard",
		Subtasks: []types.SubTask{
			types.SubTask{
				ScoringType: "sum",
				Score:       100,
				Cases:       c,
			},
		},
	}, nil
}

type dumbLang struct {
}

func (d *dumbLang) Get(name string, t language.Type) language.ExecParam {
	const pathEnv = "PATH=/usr/local/bin:/usr/bin:/bin"

	switch t {
	case language.TypeCompile:
		return language.ExecParam{
			Args: []string{"/usr/bin/g++", "-O2", "-o", "a", "a.cc"},
			Env:  []string{pathEnv},

			SourceFileName:    "a.cc",
			CompiledFileNames: []string{"a"},

			TimeLimit:   10 * time.Second,
			MemoryLimit: stypes.Size(512 << 20),
			ProcLimit:   100,
			OutputLimit: stypes.Size(64 << 10),
		}

	case language.TypeExec:
		return language.ExecParam{
			Args: []string{"a"},
			Env:  []string{pathEnv},

			SourceFileName:    "a.cc",
			CompiledFileNames: []string{"a"},

			TimeLimit:   time.Second,
			MemoryLimit: stypes.Size(256 << 20),
			ProcLimit:   1,
			OutputLimit: stypes.Size(64 << 10),
		}

	default:
		return language.ExecParam{}
	}
}
