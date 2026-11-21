//go:build windows

package pawnio

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

/*
The library PawnIO's installer puts in its own directory under Program Files
(PawnIOLib.h, LGPL-2.1, loaded at run time and not linked). Found once.
*/
var (
	libOnce sync.Once
	lib     *windows.LazyDLL
	libErr  error
)

func library() (*windows.LazyDLL, error) {
	libOnce.Do(func() {
		dir, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
		if err != nil {
			libErr = fmt.Errorf("%w: %w", ErrNoPawnIO, err)
			return
		}
		path := filepath.Join(dir, "PawnIO", "PawnIOLib.dll")
		if _, err := os.Stat(path); err != nil {
			libErr = fmt.Errorf("%w: %s", ErrNoPawnIO, path)
			return
		}
		l := windows.NewLazyDLL(path)
		if err := l.Load(); err != nil {
			libErr = fmt.Errorf("%w: %w", ErrNoPawnIO, err)
			return
		}
		lib = l
	})
	return lib, libErr
}

// HRESULTs PawnIO answers with.
const (
	eAccessDenied = 0x80070005
	eNotSupported = 0x80070032 // HRESULT_FROM_WIN32(ERROR_NOT_SUPPORTED)
)

// hresult turns a call's HRESULT into an error, nil for success.
func hresult(r uintptr) error {
	code := uint32(r) //nolint:gosec // an HRESULT is 32 bits
	switch {
	case int32(code) >= 0: //nolint:gosec // the sign is the success bit
		return nil
	case code == eAccessDenied:
		return ErrDenied
	case code == eNotSupported:
		return errors.New("the module does not support this system")
	default:
		return fmt.Errorf("HRESULT %#08x", code)
	}
}

// executor is a PawnIO handle with one module loaded.
type executor struct {
	mu     sync.Mutex
	lib    *windows.LazyDLL
	handle uintptr
}

func open(module string) (Executor, error) {
	l, err := library()
	if err != nil {
		return nil, err
	}
	dir := ModuleDir
	if dir == "" {
		exe, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("finding %s: %w", module, err)
		}
		dir = filepath.Dir(exe)
	}
	blob, err := os.ReadFile(filepath.Join(dir, filepath.Base(module)))
	if err != nil {
		return nil, fmt.Errorf("%w: %s in %s", ErrNoModule, module, dir)
	}
	var h uintptr
	r, _, _ := l.NewProc("pawnio_open").Call(uintptr(unsafe.Pointer(&h))) //nolint:gosec // the library's out-parameter
	if err := hresult(r); err != nil {
		return nil, errAt("open", err)
	}
	r, _, _ = l.NewProc("pawnio_load").Call(h, uintptr(unsafe.Pointer(&blob[0])), uintptr(len(blob))) //nolint:gosec // the blob, read by the call
	if err := hresult(r); err != nil {
		_, _, _ = l.NewProc("pawnio_close").Call(h)
		return nil, errAt("loading "+module, err)
	}
	return &executor{lib: l, handle: h}, nil
}

func (e *executor) Execute(function string, in []uint64, out int) ([]uint64, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.handle == 0 {
		return nil, errAt(function, os.ErrClosed)
	}
	name, err := windows.BytePtrFromString(function)
	if err != nil {
		return nil, errAt(function, err)
	}
	var inPtr, outPtr uintptr
	if len(in) > 0 {
		inPtr = uintptr(unsafe.Pointer(&in[0])) //nolint:gosec // kept alive across the call below
	}
	result := make([]uint64, max(out, 1))
	outPtr = uintptr(unsafe.Pointer(&result[0])) //nolint:gosec // kept alive across the call below
	var got uintptr
	r, _, _ := e.lib.NewProc("pawnio_execute").Call(
		e.handle, uintptr(unsafe.Pointer(name)), inPtr, uintptr(len(in)), outPtr, uintptr(out), uintptr(unsafe.Pointer(&got))) //nolint:gosec // the library's arguments
	if err := hresult(r); err != nil {
		return nil, errAt(function, err)
	}
	runtime.KeepAlive(in)
	runtime.KeepAlive(result)
	runtime.KeepAlive(name)
	return result[:min(int(got), out)], nil //nolint:gosec // at most out
}

func (e *executor) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.handle == 0 {
		return nil
	}
	r, _, _ := e.lib.NewProc("pawnio_close").Call(e.handle)
	e.handle = 0
	return hresult(r)
}

/*
Lock takes a named global mutex, the convention by which programs that touch
the same bus take turns: "Global\Access_SMBUS.HTP.Method" for the SMBus and
"Global\Access_ISABUS.HTP.Method" for the Super I/O chip, as OpenRGB,
LibreHardwareMonitor and HWiNFO name them. The returned function releases it,
and is called from the goroutine that took it: the goroutine is held to its
thread until then.
*/
func Lock(name string) (func(), error) {
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err //nolint:wrapcheck // a constant name
	}
	/*
		Opened, asking only to wait on it, before it is created: the mutex
		usually exists already, made by a service running as SYSTEM
		(OpenRGB's, LibreHardwareMonitor's), and its security refuses the
		full access CreateMutex asks for, even to an administrator (spec
		044's bench run). Waiting and releasing need no more.
	*/
	h, err := windows.OpenMutex(windows.SYNCHRONIZE, false, n)
	if err != nil {
		h, err = windows.CreateMutex(nil, false, n)
		if err != nil && h == 0 {
			return nil, fmt.Errorf("the %s mutex: %w", name, err)
		}
	}
	/*
		A mutex is owned by the thread that waited on it, and only that
		thread can release it: a goroutine that moved to another thread
		before releasing it would fail to, and the first thread would hold
		the bus until it ended, every other program and goroutine timing out
		(#120's live run). So the goroutine stays on its thread from the wait
		to the release, which the caller makes from the same goroutine.
	*/
	runtime.LockOSThread()
	ev, err := windows.WaitForSingleObject(h, lockWait)
	fail := func(e error) (func(), error) {
		runtime.UnlockOSThread()
		_ = windows.CloseHandle(h)
		return nil, e
	}
	if err == nil && ev == uint32(windows.WAIT_TIMEOUT) {
		return fail(fmt.Errorf("the %s mutex: %w", name, ErrBusy))
	}
	if err != nil {
		return fail(fmt.Errorf("waiting for the %s mutex: %w", name, err))
	}
	if ev != windows.WAIT_OBJECT_0 && ev != windows.WAIT_ABANDONED {
		return fail(fmt.Errorf("waiting for the %s mutex: wait result %#x", name, ev))
	}
	return func() {
		_ = windows.ReleaseMutex(h)
		runtime.UnlockOSThread()
		_ = windows.CloseHandle(h)
	}, nil
}

/*
lockWait bounds the wait for a bus mutex. Another program holds one for a
transaction, milliseconds; one held longer is a program that does not let go,
which is reported rather than waited on for ever (spec 044's bench run).
*/
const lockWait = 2000 // milliseconds

// errAt names the function a failure was in.
func errAt(function string, err error) error {
	return fmt.Errorf("PawnIO %s: %w", function, err)
}
