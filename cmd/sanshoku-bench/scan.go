package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/all"
	"github.com/ushineko/sanshoku/support"
)

// scan lists every candidate with the capabilities of the opened device and
// its support tier. Exit 1 when a driver failed; a device that fails to open
// is printed with its reason and is not a failure of the scan.
func scan(ctx context.Context, args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(errOut)
	driver := fs.String("driver", "", "restrict to one driver")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	ds, err := drivers(*driver)
	if err != nil {
		writeln(errOut, err)
		return 2
	}

	found, scanErr := sanshoku.Scan(ctx, ds...)
	entries := all.Support()

	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	writeln(tw, "DRIVER\tDEVICE\tPATH\tCAPABILITIES\tTIER")
	var notes []string
	for _, c := range found {
		entry, known := entryFor(entries, c)
		caps := "-"
		dev, err := open(ctx, c)
		switch {
		case errors.Is(err, sanshoku.ErrUnsupported):
			caps = "unsupported"
		case err != nil:
			caps = "open failed"
			note := fmt.Sprintf("%s: %v", c.Identity, err)
			if sanshoku.IsPermission(err) {
				note += "\n    " + permissionHint(c.Identity)
			}
			notes = append(notes, note)
		default:
			if names := sanshoku.Capabilities(dev); len(names) > 0 {
				caps = strings.Join(names, ",")
			}
			_ = dev.Close()
		}
		writef(tw, "%s\t%s\t%s\t%s\t%s\n", c.Driver, c.Identity, shownPath(c.Identity), caps, tierWords(entry, known))
		if known && entry.Tier == support.Expected {
			notes = append(notes, fmt.Sprintf("%s: %s", c.Identity, reportHint(c)))
		}
	}
	_ = tw.Flush()
	writef(out, "%d found\n", len(found))
	for _, n := range notes {
		writeln(out, "  "+n)
	}
	if scanErr != nil {
		writef(errOut, "scan: %v\n", scanErr)
		return 1
	}
	return 0
}
