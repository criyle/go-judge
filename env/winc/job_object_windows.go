package winc

import (
	"time"
	"unsafe"

	"github.com/criyle/go-judge/envexec"
	"golang.org/x/sys/windows"
)

func createJobObject(envLimit envexec.Limit) (windows.Handle, error) {
	// create job object
	hJob, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}

	// define job object limits
	var limit windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION

	// time limits: 100 nanoseconds / unit
	if envLimit.Time > 0 {
		limit.BasicLimitInformation.LimitFlags |= windows.JOB_OBJECT_LIMIT_JOB_TIME
		limit.BasicLimitInformation.PerJobUserTimeLimit = int64(envLimit.Time.Round(100*time.Nanosecond).Nanoseconds() / 100)
	}

	// memory limits: byte
	if envLimit.Memory > 0 {
		limit.BasicLimitInformation.LimitFlags |= windows.JOB_OBJECT_LIMIT_JOB_MEMORY
		limit.JobMemoryLimit = uintptr(envLimit.Memory)
	}

	// process limit
	if envLimit.Proc > 0 {
		limit.BasicLimitInformation.LimitFlags |= windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS
		limit.BasicLimitInformation.ActiveProcessLimit = uint32(envLimit.Proc)
	}

	// additional limitations
	limit.BasicLimitInformation.LimitFlags |= windows.JOB_OBJECT_LIMIT_DIE_ON_UNHANDLED_EXCEPTION | windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(hJob, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limit)), uint32(unsafe.Sizeof(limit)))
	if err != nil {
		windows.CloseHandle(hJob)
		return 0, err
	}

	// ui restrictions
	var uiRestriction windows.JOBOBJECT_BASIC_UI_RESTRICTIONS
	uiRestriction.UIRestrictionsClass = windows.JOB_OBJECT_UILIMIT_EXITWINDOWS |
		windows.JOB_OBJECT_UILIMIT_DESKTOP |
		windows.JOB_OBJECT_UILIMIT_DISPLAYSETTINGS |
		windows.JOB_OBJECT_UILIMIT_GLOBALATOMS |
		windows.JOB_OBJECT_UILIMIT_HANDLES |
		windows.JOB_OBJECT_UILIMIT_READCLIPBOARD |
		windows.JOB_OBJECT_UILIMIT_SYSTEMPARAMETERS |
		windows.JOB_OBJECT_UILIMIT_WRITECLIPBOARD

	_, err = windows.SetInformationJobObject(hJob, windows.JobObjectBasicUIRestrictions, uintptr(unsafe.Pointer(&uiRestriction)), uint32(unsafe.Sizeof(uiRestriction)))
	if err != nil {
		windows.CloseHandle(hJob)
		return 0, err
	}
	return hJob, nil
}
