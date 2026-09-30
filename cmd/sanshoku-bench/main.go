/*
sanshoku-bench is the hardware testbench: the module's integration test and
its correctness oracle. scan, read and verify are read-only; screen --yes is
the one write; support prints the support table. Spec 001 R7.

Output never carries a Bluetooth address or a serial number: a device prints
as its kernel name and vendor:product, and its node path.
*/
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
)

const usage = `usage: sanshoku-bench <verb> [flags]

  scan                 list every device the drivers find, with capabilities and tier
  read                 open every device and take every read-only reading
                       --json  --driver NAME  --repeat N  --interval D
  verify               cross-check readings against liquidctl and solaar when on PATH
  support              print the support table; --markdown prints docs/devices.md
  screen --yes         push a test image to a screen and return it to its readout
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// run dispatches a verb and returns the exit status: 0 on success, 1 on a
// failure or disagreement, 2 on a usage error.
func run(ctx context.Context, args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		write(errOut, usage)
		return 2
	}
	verb, rest := args[0], args[1:]
	switch verb {
	case "scan":
		return scan(ctx, rest, out, errOut)
	case "read":
		return read(ctx, rest, out, errOut)
	case "verify":
		return verify(ctx, rest, out, errOut)
	case "support":
		return supportTable(rest, out, errOut)
	case "screen":
		return screenPush(rest, out, errOut)
	case "help", "-h", "--help":
		write(out, usage)
		return 0
	default:
		writef(errOut, "sanshoku-bench: unknown verb %q\n\n%s", verb, usage)
		return 2
	}
}

// writef, writeln and write print to the terminal. A failed write to a
// terminal has nowhere better to be reported, so its error is dropped here
// once rather than at every call.
func writef(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }
func writeln(w io.Writer, a ...any)               { _, _ = fmt.Fprintln(w, a...) }
func write(w io.Writer, a ...any)                 { _, _ = fmt.Fprint(w, a...) }
