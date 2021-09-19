package filestore

import (
	"container/heap"
	"os"
	"sync"
	"time"

	"github.com/criyle/go-judge/envexec"
)

var (
	_ FileStore      = &Timeout{}
	_ heap.Interface = &Timeout{}
)

// Timeout is a file system with a maximun TTL
type Timeout struct {
	mu sync.Mutex
	FileStore
	timeout   time.Duration
	files     []timeoutFile
	idToIndex map[string]int
}

type timeoutFile struct {
	id   string
	time time.Time
}

// NewTimeout creates a timeout file system with maximun TTL for a file
func NewTimeout(fs FileStore, timeout time.Duration, checkInterval time.Duration) FileStore {
	t := &Timeout{
		FileStore: fs,
		timeout:   timeout,
		files:     make([]timeoutFile, 0),
		idToIndex: make(map[string]int),
	}
	go t.checkTimeoutLoop(checkInterval)
	return t
}

func (t *Timeout) checkTimeoutLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for {
		t.checkTimeoutAndRemove()
		<-ticker.C
	}
}

func (t *Timeout) checkTimeoutAndRemove() {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	for len(t.files) > 0 && t.files[0].time.Add(t.timeout).Before(now) {
		f := t.files[0]
		t.FileStore.Remove(f.id)
		heap.Pop(t)
	}
}

func (t *Timeout) Len() int {
	return len(t.files)
}

func (t *Timeout) Less(i, j int) bool {
	return t.files[i].time.Before(t.files[j].time)
}

func (t *Timeout) Swap(i, j int) {
	t.files[i], t.files[j] = t.files[j], t.files[i]
	t.idToIndex[t.files[i].id] = i
	t.idToIndex[t.files[j].id] = j
}

func (t *Timeout) Push(x interface{}) {
	e := x.(timeoutFile)
	t.files = append(t.files, e)
	t.idToIndex[e.id] = len(t.files) - 1
}

func (t *Timeout) Pop() interface{} {
	e := t.files[len(t.files)-1]
	t.files = t.files[:len(t.files)-1]
	delete(t.idToIndex, e.id)
	return e
}

func (t *Timeout) Add(name, path string) (string, error) {
	// try add to file store underlying
	id, err := t.FileStore.Add(name, path)
	if err != nil {
		return "", err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	f := timeoutFile{id, time.Now()}
	heap.Push(t, f)

	return id, nil
}

func (t *Timeout) Remove(id string) bool {
	success := t.FileStore.Remove(id)

	t.mu.Lock()
	defer t.mu.Unlock()

	index, ok := t.idToIndex[id]
	if !ok {
		return success
	}
	heap.Remove(t, index)
	return success
}

func (t *Timeout) Get(id string) (string, envexec.File) {
	name, file := t.FileStore.Get(id)

	t.mu.Lock()
	defer t.mu.Unlock()

	index, ok := t.idToIndex[id]
	if !ok {
		return name, file
	}
	t.files[index].time = time.Now()
	heap.Fix(t, index)

	return name, file
}

func (t *Timeout) New() (*os.File, error) {
	return t.FileStore.New()
}
