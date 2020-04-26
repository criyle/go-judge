package winc

import "golang.org/x/sys/windows"

// JOBOBJECT_ASSOCIATE_COMPLETION_PORT defines IOCP for job object
type JOBOBJECT_ASSOCIATE_COMPLETION_PORT struct {
	CompletionKey  windows.Handle
	CompletionPort windows.Handle
}

// JOBOBJECT_BASIC_ACCOUNTING_INFORMATION defines basic accounting information for a job object
type JOBOBJECT_BASIC_ACCOUNTING_INFORMATION struct {
	TotalUserTime             int64
	TotalKernelTime           int64
	ThisPeriodTotalUserTime   int64
	ThisPeriodTotalKernelTime int64
	TotalPageFaultCount       uint32
	TotalProcesses            uint32
	ActiveProcesses           uint32
	TotalTerminatedProcess    uint32
}

// JobObject Information class
const (
	JobObjectBasicAccountingInformation = iota + 1
	JobObjectBasicLimitInformation
	JobObjectBasicProcessIDList
	JobObjectBasicUIRestrictions
	JobObjectSecurityLimitInformation
	JobObjectEndOfJobTimeInformation
	JobObjectAssociateCompletionPortInformation
	JobObjectBasicAndIoAccountingInformation
	JobObjectExtendedLimitInformation
	_
	JobObjectGroupInformation
	JobObjectNotificationLimitInformation
	JobObjectLimitViolationInformation
	JobObjectGroupInformationEx
	JobObjectCPURateControlInformation
)

// JobObject Messages https://chromium.googlesource.com/chromium/deps/mozprocess/+/11d11bebc8517dcedec71f377cbec07fb91a3b1f/mozprocess/winprocess.py
// https://docs.microsoft.com/en-ca/windows/win32/api/winnt/ns-winnt-jobobject_associate_completion_port?redirectedfrom=MSDN
const (
	JOB_OBJECT_MSG_END_OF_JOB_TIME = iota + 1
	JOB_OBJECT_MSG_END_OF_PROCESS_TIME
	JOB_OBJECT_MSG_ACTIVE_PROCESS_LIMIT
	JOB_OBJECT_MSG_ACTIVE_PROCESS_ZERO
	JOB_OBJECT_MSG_NEW_PROCESS
	JOB_OBJECT_MSG_EXIT_PROCESS
	JOB_OBJECT_MSG_ABNORMAL_EXIT_PROCESS
	JOB_OBJECT_MSG_PROCESS_MEMORY_LIMIT
	JOB_OBJECT_MSG_JOB_MEMORY_LIMIT
)

// https://docs.microsoft.com/en-us/windows/win32/api/securitybaseapi/nf-securitybaseapi-createrestrictedtoken
// CreateRestrictedToken
// CreateRestrictedToken Flags
const (
	DISABLE_MAX_PRIVILEGE = 1 << iota
	SANDBOX_INERT
	LUA_TOKEN
	WRITE_RESTRICTED
)

// https://docs.microsoft.com/en-ca/windows/win32/winstation/desktop-security-and-access-rights
const (
	DESKTOP_READOBJECTS windows.ACCESS_MASK = 1 << iota
	DESKTOP_CREATEWINDOW
	DESKTOP_CREATEMENU
	DESKTOP_HOOKCONTROL
	DESKTOP_JOURNALRECORD
	DESKTOP_JOURNALPLAYBACK
	DESKTOP_ENUMERATE
	DESKTOP_WRITEOBJECTS
	DESKTOP_SWITCHDESKTOP // 0x0100L
)

// permissions
const (
	DELETE windows.ACCESS_MASK = 1 << (iota + 16)
	READ_CONTROL
	WRITE_DAC
	WRITE_OWNER
	SYNCHRONIZE
)

// https://docs.microsoft.com/en-us/windows/win32/secauthz/standard-access-rights
const (
	GENERIC_READ  = DESKTOP_ENUMERATE | DESKTOP_READOBJECTS | READ_CONTROL
	GENERIC_WRITE = DESKTOP_CREATEMENU | DESKTOP_CREATEWINDOW |
		DESKTOP_HOOKCONTROL | DESKTOP_JOURNALPLAYBACK | DESKTOP_JOURNALRECORD |
		DESKTOP_WRITEOBJECTS | READ_CONTROL
	GENERIC_EXECUTE = DESKTOP_SWITCHDESKTOP | READ_CONTROL
	GENERIC_ALL     = DESKTOP_CREATEMENU | DESKTOP_CREATEWINDOW |
		DESKTOP_ENUMERATE | DESKTOP_HOOKCONTROL | DESKTOP_JOURNALPLAYBACK |
		DESKTOP_JOURNALRECORD | DESKTOP_READOBJECTS | DESKTOP_SWITCHDESKTOP |
		DESKTOP_WRITEOBJECTS | READ_CONTROL | WRITE_DAC | WRITE_OWNER
)

// HDESK is handle for desktop
type HDESK windows.Handle

// HWINSTA is handle for windows station
type HWINSTA windows.Handle

// Mandatory Level Sids
const (
	SID_SYSTEM_MANDATORY_LEVEL    = "S-1-16-16384"
	SID_HIGH_MANDATORY_LEVEL      = "S-1-16-12288"
	SID_MEDIUM_MANDATORY_LEVEL    = "S-1-16-8192"
	SID_LOW_MANDATORY_LEVEL       = "S-1-16-4096"
	SID_UNTRUSTED_MANDATORY_LEVEL = "S-1-16-0"
)

//sys CreateRestrictedToken(existingToken windows.Token, flags uint32, disableSidCount uint32, sidsToDisable *windows.SIDAndAttributes, deletePrivilegeCount uint32, privilegesToDelete *windows.SIDAndAttributes, restrictedSidCount uint32, sidToRestrict *windows.SIDAndAttributes, newTokenHandle *windows.Token) (err error) = advapi32.CreateRestrictedToken
//sys GetThreadDesktop(threadID uint32) (h HDESK) = user32.GetThreadDesktop
//sys GetProcessWindowStation() (h HWINSTA) = user32.GetProcessWindowStation
//sys CreateDesktop(lpszDesktop *uint16, lpszDevice *uint16, pDevmode uintptr, dwFlags uint32, dwDesiredAccess windows.ACCESS_MASK, lpsa *windows.SecurityAttributes) (h HDESK, err error) = user32.CreateDesktopW
//sys CloseDesktop(hDesktop HDESK) (err error) = user32.CloseDesktop
//sys QueryInformationJobObject(job windows.Handle, JobObjectInformationClass uint32, JobObjectInformation uintptr, JobObjectInformationLength uint32, lpReturnLength *uint32) (ret int, err error) = kernel32.QueryInformationJobObject
