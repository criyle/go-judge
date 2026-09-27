// Command seccomp-convert converts the Moby seccomp profile into the
// architecture-aware seccomp YAML format used by go-judge.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/elastic/go-seccomp-bpf/arch"
)

type profile struct {
	DefaultAction   string  `json:"defaultAction"`
	DefaultErrnoRet int     `json:"defaultErrnoRet"`
	Syscalls        []group `json:"syscalls"`
}

type group struct {
	Names  []string `json:"names"`
	Action string   `json:"action"`
	Args   []arg    `json:"args"`
	// Moby uses these for runtime capability and architecture selection. The
	// converter drops capability-dependent rules because go-judge drops
	// capabilities before exec, and resolves architecture selectors while
	// generating each architecture section.
	Includes *selector `json:"includes"`
	Excludes *selector `json:"excludes"`
}

type selector struct {
	Caps   []string `json:"caps"`
	Arches []string `json:"arches"`
}

type arg struct {
	Index uint32 `json:"index"`
	Value uint64 `json:"value"`
	Op    string `json:"op"`
}

type targetArchitecture struct {
	name       string
	mobyArches map[string]bool
}

type generatedGroup struct {
	key   string
	names []string
	name  string
	arg   *arg
}

var targetArchitectures = []targetArchitecture{
	{name: "x86_64", mobyArches: map[string]bool{"amd64": true}},
	{name: "i386", mobyArches: map[string]bool{"x86": true}},
	{name: "aarch64", mobyArches: map[string]bool{"arm64": true}},
	{name: "arm", mobyArches: map[string]bool{"arm": true}},
}

// These syscall groups are intentionally stricter than the Moby baseline.
// Every entry is absent from the current compiler/runtime observation, and the
// groups are excluded because they add authority or kernel attack surface that
// submitted programs do not need.
var deniedSyscallGroups = []struct {
	reason string
	names  []string
}{
	{
		// Direct process inspection or modification; these expose another
		// process's memory, credentials, or execution state.
		reason: "cross-process inspection and control",
		names:  []string{"get_robust_list", "process_mrelease", "process_vm_readv", "process_vm_writev", "ptrace"},
	},
	{
		// Alternate executable loading and anonymous executable-file staging
		// broaden fileless payload and loader behavior.
		reason: "fileless executable loading",
		names:  []string{"execveat", "memfd_create", "memfd_secret"},
	},
	{
		// Legacy x86 interfaces are not needed by the supported x86-64 corpus
		// and have a disproportionate amount of compatibility surface.
		reason: "legacy x86 execution state",
		names:  []string{"get_thread_area", "modify_ldt", "set_thread_area"},
	},
	{
		// Device-node creation can expose host devices if capabilities or
		// mounts are ever configured less restrictively.
		reason: "device-node creation",
		names:  []string{"mknod", "mknodat"},
	},
	{
		// Filesystem event monitoring can attach a process to activity outside
		// the files it otherwise needs to access.
		reason: "filesystem event attachment",
		names:  []string{"fanotify_mark"},
	},
	{
		// These can signal, identify, reprioritize, or otherwise affect other
		// processes when the kernel's credential checks permit it. The init's
		// cleanup path requires kill, so it is retained in the base policy.
		reason: "cross-process signaling and scheduling",
		names:  []string{"getpgid", "getsid", "rt_sigqueueinfo", "rt_tgsigqueueinfo", "sched_setattr", "sched_setparam", "tkill"},
	},
	{
		// Legacy asynchronous I/O is unused and adds another kernel I/O path.
		reason: "legacy asynchronous I/O",
		names:  []string{"io_cancel", "io_destroy", "io_getevents", "io_pgetevents", "io_setup", "io_submit"},
	},
	{
		// These provide alternate filesystem-handle and mount-topology
		// interfaces; ordinary openat/newfstatat cover the observed corpus.
		reason: "alternate filesystem and mount interfaces",
		names:  []string{"listmount", "name_to_handle_at", "openat2", "statmount"},
	},
	{
		// Kernel-mediated descriptor-to-descriptor transfer is unnecessary for
		// the observed workloads and complicates copy/data-flow auditing.
		reason: "kernel-mediated data transfer",
		names:  []string{"copy_file_range", "sendfile", "splice", "tee", "vmsplice"},
	},
}

var deniedSyscalls = func() map[string]bool {
	denied := make(map[string]bool)
	for _, group := range deniedSyscallGroups {
		for _, name := range group.names {
			denied[name] = true
		}
	}
	// personality is kept separate because it changes the calling process's
	// execution ABI and is already covered by the integration security probe.
	denied["personality"] = true
	return denied
}()

func main() {
	in := flag.String("input", "seccomp/moby-default.json", "Moby profile")
	out := flag.String("output", "seccomp/moby.yaml", "architecture-aware Elastic seccomp YAML profile")
	flag.Parse()

	f, err := os.Open(*in)
	if err != nil {
		fatal(err)
	}
	defer f.Close()

	var p profile
	if err := json.NewDecoder(f).Decode(&p); err != nil {
		fatal(err)
	}

	o, err := os.Create(*out)
	if err != nil {
		fatal(err)
	}
	defer func() {
		if err := o.Close(); err != nil {
			fatal(err)
		}
	}()

	policies := make(map[string][]generatedGroup, len(targetArchitectures))
	var order []string
	for _, target := range targetArchitectures {
		info, err := arch.GetInfo(target.name)
		if err != nil {
			fatal(err)
		}
		groups := generatePolicy(p.Syscalls, target, info)
		policies[target.name] = groups
		for _, group := range groups {
			if !contains(order, group.key) {
				order = append(order, group.key)
			}
		}
	}

	base := make(map[string]generatedGroup)
	for _, key := range order {
		if group, ok := commonGroup(key, policies); ok {
			base[key] = group
		}
	}

	fmt.Fprintln(o, "default_action: errno")
	fmt.Fprintln(o, "base:")
	fmt.Fprintln(o, "  syscalls:")
	writePolicyGroups(o, commonGroups(order, base), "    ")
	fmt.Fprintln(o, "architectures:")
	for _, target := range targetArchitectures {
		fmt.Fprintf(o, "  %s:\n    syscalls:\n", target.name)
		writePolicyGroups(o, additionalGroups(order, policies[target.name], base), "      ")
	}
}

func generatePolicy(groups []group, target targetArchitecture, info *arch.Info) []generatedGroup {
	var generated []generatedGroup
	for groupIndex, g := range groups {
		if !includeGroup(g, target) || g.Action != "SCMP_ACT_ALLOW" || len(g.Names) == 0 {
			continue
		}
		if len(g.Args) == 0 {
			names := validNames(g.Names, info)
			names = withoutName(names, "socketcall")
			if len(names) > 0 {
				generated = append(generated, generatedGroup{
					key:   fmt.Sprintf("group-%d", groupIndex),
					names: names,
				})
			}
			continue
		}
		for _, name := range validNames(g.Names, info) {
			// The container init and the CLONE_INTO_CGROUP path both require
			// unrestricted clone. The init filter is installed only after
			// container setup, so retain namespace-capable clone calls.
			if name == "clone" && len(g.Args) > 0 {
				continue
			}
			for _, a := range g.Args {
				argCopy := a
				generated = append(generated, generatedGroup{
					key:  fmt.Sprintf("group-%d-%s-%d-%d-%s", groupIndex, name, a.Index, a.Value, a.Op),
					name: name,
					arg:  &argCopy,
				})
			}
		}
	}

	// Moby's profile allows socketcall as a name, but that would allow a
	// 32-bit process to reach socket(AF_ALG, ...). Permit all socketcall
	// operations except SYS_SOCKET (1), which is the Copy Fail entry point.
	if _, ok := info.SyscallNames["socketcall"]; ok {
		a := arg{Index: 0, Value: 1, Op: "SCMP_CMP_GT"}
		generated = append(generated, generatedGroup{key: "socketcall", name: "socketcall", arg: &a})
	}
	for _, name := range []string{"clone", "clone3", "kill", "unshare"} {
		if _, ok := info.SyscallNames[name]; ok {
			generated = append(generated, generatedGroup{key: "init-" + name, names: []string{name}})
		}
	}
	return generated
}

func commonGroup(key string, policies map[string][]generatedGroup) (generatedGroup, bool) {
	var common generatedGroup
	first := true
	for _, target := range targetArchitectures {
		group, ok := findGroup(policies[target.name], key)
		if !ok {
			return generatedGroup{}, false
		}
		if first {
			common = group
			first = false
			continue
		}
		if common.arg != nil || group.arg != nil {
			if !sameArgGroup(common, group) {
				return generatedGroup{}, false
			}
			continue
		}
		common.names = intersect(common.names, group.names)
	}
	if common.arg == nil && len(common.names) == 0 {
		return generatedGroup{}, false
	}
	return common, true
}

func commonGroups(order []string, base map[string]generatedGroup) []generatedGroup {
	groups := make([]generatedGroup, 0, len(base))
	for _, key := range order {
		if group, ok := base[key]; ok {
			groups = append(groups, group)
		}
	}
	return groups
}

func additionalGroups(order []string, groups []generatedGroup, base map[string]generatedGroup) []generatedGroup {
	byKey := make(map[string]generatedGroup, len(groups))
	for _, group := range groups {
		byKey[group.key] = group
	}
	additional := make([]generatedGroup, 0, len(groups))
	for _, key := range order {
		group, ok := byKey[key]
		if !ok {
			continue
		}
		common, isCommon := base[key]
		if !isCommon || group.arg != nil {
			if !isCommon || !sameArgGroup(group, common) {
				additional = append(additional, group)
			}
			continue
		}
		group.names = difference(group.names, common.names)
		if len(group.names) > 0 {
			additional = append(additional, group)
		}
	}
	return additional
}

func writePolicyGroups(w io.Writer, groups []generatedGroup, indent string) {
	for _, group := range groups {
		if group.arg == nil {
			writeNames(w, group.names, indent)
		} else {
			writeArg(w, group.name, *group.arg, indent)
		}
	}
}

func findGroup(groups []generatedGroup, key string) (generatedGroup, bool) {
	for _, group := range groups {
		if group.key == key {
			return group, true
		}
	}
	return generatedGroup{}, false
}

func sameArgGroup(a, b generatedGroup) bool {
	return a.arg != nil && b.arg != nil && a.name == b.name && *a.arg == *b.arg
}

func intersect(a, b []string) []string {
	set := make(map[string]bool, len(b))
	for _, name := range b {
		set[name] = true
	}
	result := make([]string, 0, len(a))
	for _, name := range a {
		if set[name] {
			result = append(result, name)
		}
	}
	return result
}

func difference(a, b []string) []string {
	set := make(map[string]bool, len(b))
	for _, name := range b {
		set[name] = true
	}
	result := make([]string, 0, len(a))
	for _, name := range a {
		if !set[name] {
			result = append(result, name)
		}
	}
	return result
}

func withoutName(names []string, unwanted string) []string {
	result := make([]string, 0, len(names))
	for _, name := range names {
		if name != unwanted {
			result = append(result, name)
		}
	}
	return result
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func includeGroup(g group, target targetArchitecture) bool {
	if g.Includes != nil && len(g.Includes.Caps) > 0 {
		return false
	}
	if g.Excludes != nil && len(g.Excludes.Caps) > 0 && len(g.Args) == 0 {
		return false
	}
	if g.Includes != nil && len(g.Includes.Arches) > 0 {
		matched := false
		for _, name := range g.Includes.Arches {
			if target.mobyArches[name] {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if g.Excludes != nil {
		for _, name := range g.Excludes.Arches {
			if target.mobyArches[name] {
				return false
			}
		}
	}
	return true
}

func validNames(names []string, info *arch.Info) []string {
	valid := make([]string, 0, len(names))
	for _, name := range names {
		if _, ok := info.SyscallNames[name]; ok && !deniedSyscalls[name] {
			valid = append(valid, name)
		}
	}
	return valid
}

func writeNames(w io.Writer, names []string, indent string) {
	fmt.Fprintf(w, "%s- names:\n", indent)
	for _, name := range names {
		fmt.Fprintf(w, "%s    - %s\n", indent, name)
	}
	fmt.Fprintf(w, "%s  action: allow\n", indent)
}

func writeArg(w io.Writer, name string, a arg, indent string) {
	op := map[string]string{
		"SCMP_CMP_EQ":        "Equal",
		"SCMP_CMP_LT":        "LessThan",
		"SCMP_CMP_LE":        "LessOrEqual",
		"SCMP_CMP_GT":        "GreaterThan",
		"SCMP_CMP_GE":        "GreaterOrEqual",
		"SCMP_CMP_MASKED_EQ": "BitsSet",
	}[a.Op]
	if op == "" {
		fatal(fmt.Errorf("unsupported Moby comparison: %s", a.Op))
	}
	fmt.Fprintf(w, "%s- names_with_args:\n%s    - name: %s\n%s      arguments:\n%s        - position: %d\n%s          operation: %s\n%s          value: %d\n%s  action: allow\n", indent, indent, name, indent, indent, a.Index, indent, op, indent, a.Value, indent)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
