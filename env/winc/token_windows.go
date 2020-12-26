package winc

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func createLowMandatoryLevelToken() (token windows.Token, err error) {
	// Get current process token
	var procToken windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(),
		windows.TOKEN_QUERY|windows.TOKEN_DUPLICATE|windows.TOKEN_ADJUST_DEFAULT|windows.TOKEN_ASSIGN_PRIMARY,
		&procToken); err != nil {
		return token, err
	}

	// create restricted token
	if err := CreateRestrictedToken(procToken, DISABLE_MAX_PRIVILEGE, 0, nil, 0, nil, 0, nil, &token); err != nil {
		return token, err
	}
	defer func() {
		if err != nil {
			token.Close()
		}
	}()

	lowSid, err := windows.StringToSid(SID_LOW_MANDATORY_LEVEL)
	if err != nil {
		return token, err
	}
	tml := windows.Tokenmandatorylabel{
		Label: windows.SIDAndAttributes{
			Sid:        lowSid,
			Attributes: windows.SE_GROUP_INTEGRITY,
		},
	}
	if err = windows.SetTokenInformation(token, syscall.TokenIntegrityLevel, (*byte)(unsafe.Pointer(&tml)), uint32(unsafe.Sizeof(tml))); err != nil {
		return token, err
	}
	return token, nil
}
