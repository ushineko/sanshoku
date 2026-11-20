//go:build !windows

package pawnio

// open is ErrNoPawnIO: PawnIO is Windows's.
func open(string) (Executor, error) { return nil, ErrNoPawnIO }

// Lock is a no-op where there is no PawnIO to share a bus through.
func Lock(string) (func(), error) { return func() {}, nil }
