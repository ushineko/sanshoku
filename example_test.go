package sanshoku_test

import (
	"context"
	"fmt"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/all"
	"github.com/ushineko/sanshoku/battery"
)

/*
Example is the README's "Using it" block. readme_test.go checks that the block
is this function's body, character for character, so the README's example is
one that compiles.

It has no Output comment, so go test compiles it and does not run it: running
it would open whatever is plugged in, and make test opens no device.
*/
func Example() {
	ctx := context.Background()
	found, err := sanshoku.Scan(ctx, all.Drivers()...)
	if err != nil {
		fmt.Println(err) // a driver failed; what the others found is still here
	}
	for _, c := range found {
		dev, err := c.Open(ctx)
		if err != nil {
			continue // sanshoku.IsPermission(err): the udev rule is missing
		}
		if src, ok := dev.(battery.Source); ok {
			batteries, _ := src.Batteries(ctx)
			for _, b := range batteries {
				if b.HasLevel {
					fmt.Printf("%s: %d%%\n", b.Name, b.Level)
				}
			}
		}
		_ = dev.Close()
	}
}
