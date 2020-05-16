package macsandbox

import (
	"bytes"
	"path/filepath"
	"text/template"
)

// Reference: https://github.com/chromium/chromium/blob/24d4eaa172c6a84974fe7cd4096a60a9b64abd9c/services/service_manager/sandbox/mac/common.sb
const sandboxTemplate = `(version 1)

(deny default)

; allow posix ipc
(allow ipc-posix*)

; allow execve 
(allow process-exec)

; allow fork
(allow process-fork)

; allow signal to self
(allow signal (target self))

; sysctls permitted.
(allow sysctl-read
  (sysctl-name "hw.activecpu")
  (sysctl-name "hw.busfrequency_compat")
  (sysctl-name "hw.byteorder")
  (sysctl-name "hw.cachelinesize_compat")
  (sysctl-name "hw.cpufrequency_compat")
  (sysctl-name "hw.cputype")
  (sysctl-name "hw.logicalcpu_max")
  (sysctl-name "hw.machine")
  (sysctl-name "hw.ncpu")
  (sysctl-name "hw.pagesize_compat")
  (sysctl-name "hw.physicalcpu_max")
  (sysctl-name "hw.tbfrequency_compat")
  (sysctl-name "hw.vectorunit")
  (sysctl-name "kern.hostname")
  (sysctl-name "kern.maxfilesperproc")
  (sysctl-name "kern.osrelease")
  (sysctl-name "kern.ostype")
  (sysctl-name "kern.osvariant_status")
  (sysctl-name "kern.osversion")
  (sysctl-name "kern.usrstack64")
  (sysctl-name "kern.version")
  (sysctl-name "sysctl.proc_cputype")
  (sysctl-name "kern.proc.pid.CURRENT_PID")
)

; allow read from dir
{{- range $dir := .ReadableDir }}
(allow file-read* (subpath "{{$dir}}"))
{{- end }}

; deny users
(deny file-read* (subpath "/Users"))

; allow write to dir
{{- range $dir := .WritableDir }}
(allow file-write* (subpath "{{$dir}}"))
{{- end }}

{{- if .Network }}
(allow network-outbound)
{{- end }}
`

var profileTpl = template.Must(template.New("profile").Parse(sandboxTemplate))

// Profile defines MacOS sandbox profile
type Profile struct {
	WritableDir, ReadableDir []string
	Network                  bool
}

// DefaultProfile defines the minimun default profile to run programs
var DefaultProfile = Profile{
	ReadableDir: []string{"/usr/lib"},
}

// Build generates the sandbox profile
func (p *Profile) Build() (string, error) {
	var buf bytes.Buffer

	realRead, err := getRealPaths(p.ReadableDir)
	if err != nil {
		return "", err
	}
	realWrite, err := getRealPaths(p.WritableDir)
	if err != nil {
		return "", err
	}
	realProfile := Profile{
		WritableDir: realWrite,
		ReadableDir: realRead,
		Network:     p.Network,
	}

	if err := profileTpl.Execute(&buf, realProfile); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func getRealPath(s string) (string, error) {
	return filepath.EvalSymlinks(s)
}

func getRealPaths(original []string) ([]string, error) {
	ret := make([]string, 0, len(original))
	for _, s := range original {
		t, err := getRealPath(s)
		if err != nil {
			return nil, err
		}
		ret = append(ret, t)
	}
	return ret, nil
}
