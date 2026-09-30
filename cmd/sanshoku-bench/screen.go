package main

import (
	"flag"
	"io"
)

// screenPush is the bench's one write: push a generated test image to every
// screen found, wait, and return each to its readout. It refuses without
// --yes. No driver in the module has a screen yet; spec 005 lands the first.
func screenPush(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("screen", flag.ContinueOnError)
	fs.SetOutput(errOut)
	yes := fs.Bool("yes", false, "allow writing to the device's display")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if !*yes {
		writeln(errOut, "screen writes to a device's display; pass --yes to allow it")
		return 2
	}
	writeln(out, "no screen driver yet")
	return 0
}
