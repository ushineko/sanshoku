package lhm

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// process is LibreHardwareMonitor's executable, as a process list shows it.
const process = "librehardwaremonitor.exe"

// pawnIOService is the PawnIO driver's service name.
const pawnIOService = "PawnIO"

/*
taskKey is where Task Scheduler indexes LibreHardwareMonitor's startup task:
the name its own Run On Windows Startup gives it, in the root folder. The
index under HKLM is readable without administrator rights, where Task
Scheduler's COM interface would need a COM runtime and the task's file under
System32\Tasks is not readable at all (hayami spec 042).
*/
const taskKey = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Schedule\TaskCache\Tree\LibreHardwareMonitor`

/*
ProbeHost says whether PawnIO is installed, LibreHardwareMonitor is running
and its startup task is registered.

All three are asked read-only, and none needs administrator rights: the
service manager is opened to connect and the service to query its status,
which any user may; the process list names every process, elevated ones
included; the task index is a registry key any user may read.
*/
func ProbeHost() Host {
	return Host{
		PawnIO:  serviceInstalled(pawnIOService),
		Running: processRunning(process),
		Task:    keyExists(registry.LOCAL_MACHINE, taskKey),
	}
}

// keyExists is whether a registry key can be opened to read.
func keyExists(root registry.Key, path string) bool {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	_ = k.Close()
	return true
}

func serviceInstalled(name string) bool {
	m, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseServiceHandle(m) }()
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return false
	}
	s, err := windows.OpenService(m, n, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return false
	}
	_ = windows.CloseServiceHandle(s)
	return true
}

func processRunning(exe string) bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(snap) }()
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), exe) {
			return true
		}
	}
	return false
}
