/*
Package pawnio runs functions of PawnIO modules on Windows: the signed,
scripted kernel driver through which a program reaches the SMBus and a
mainboard's Super I/O chip once WinRing0 is gone (spec 044).

PawnIO is installed on its own (pawnio.eu), and its modules are blobs signed
by its author, loaded by name from ModuleDir. This package installs neither.
PawnIO serves only a process with Administrator rights.

Two of its users ride on this package: package i2c's SMBus on Windows, and
the Super I/O access in this package. Both are written against Executor, so
their logic is tested on any system.
*/
package pawnio

import (
	"errors"
)

/*
Executor is one loaded module: Execute runs one of its functions, with up to
in's values in, and returns up to out values. Close unloads it.
*/
type Executor interface {
	Execute(function string, in []uint64, out int) ([]uint64, error)
	Close() error
}

/*
ModuleDir is where module blobs are read from ("SmbusPIIX4.bin"). Empty is the
running program's own directory, as OpenRGB keeps them. A variable rather than
an argument because the one setting serves every module a consumer loads.
*/
var ModuleDir string

// ErrNoPawnIO is a system without PawnIO: not Windows, or PawnIO not
// installed.
var ErrNoPawnIO = errors.New("PawnIO is not available")

// ErrNoModule is a module blob not found where ModuleDir says.
var ErrNoModule = errors.New("the PawnIO module is not there")

// ErrDenied is PawnIO refusing a process without Administrator rights.
var ErrDenied = errors.New("PawnIO needs Administrator rights")

/*
Open loads a module by its file name, "LpcIO.bin" for example, into a new
PawnIO executor. A module the system cannot use ("unsupported", such as the
Intel SMBus module on an AMD board) is an error.
*/
func Open(module string) (Executor, error) {
	return open(module)
}
