package winc

import (
	"os"

	"github.com/criyle/go-judge/env/pool"
	"golang.org/x/sys/windows"
)

const sdString = "S:(ML;;NW;;;LW)D:(A;;0x12019f;;;WD)"
const wdPrefix = "es"
const tmpPrefix = "es_tmp"

var _ pool.EnvBuilder = &builder{}

type builder struct {
	root  string
	token windows.Token
	sd    *windows.SECURITY_DESCRIPTOR
}

// NewBuilder returns a builder for windows container environment
func NewBuilder(root string) (pool.EnvBuilder, error) {
	sd, err := windows.SecurityDescriptorFromString(sdString)
	if err != nil {
		return nil, err
	}

	token, err := createLowMandatoryLevelToken()
	if err != nil {
		return nil, err
	}
	return &builder{
		root:  root,
		token: token,
		sd:    sd,
	}, nil
}

func (b *builder) Build() (pool.Environment, error) {
	sacl, _, err := b.sd.SACL()
	if err != nil {
		return nil, err
	}
	workDir, err := os.MkdirTemp(b.root, wdPrefix)
	if err != nil {
		return nil, err
	}
	if err := windows.SetNamedSecurityInfo(workDir, windows.SE_FILE_OBJECT,
		windows.LABEL_SECURITY_INFORMATION, nil, nil, nil, sacl); err != nil {
		return nil, err
	}
	tmpDir, err := os.MkdirTemp(b.root, tmpPrefix)
	if err != nil {
		return nil, err
	}
	if err := windows.SetNamedSecurityInfo(tmpDir, windows.SE_FILE_OBJECT,
		windows.LABEL_SECURITY_INFORMATION, nil, nil, nil, sacl); err != nil {
		return nil, err
	}

	wd, err := os.Open(workDir)
	if err != nil {
		return nil, err
	}
	return &Environment{
		root:  workDir,
		tmp:   tmpDir,
		wd:    wd,
		token: b.token,
		sd:    b.sd,
	}, nil
}
