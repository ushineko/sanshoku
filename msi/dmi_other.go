//go:build !windows

package msi

import (
	"os"
	"strings"
)

// dmiBoard is the mainboard's maker and name, from where Linux keeps them.
// The board is found here and refused at open, as PawnIO is Windows's.
func dmiBoard() (maker, name string) {
	read := func(path string) string {
		b, err := os.ReadFile(path)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	}
	return read("/sys/class/dmi/id/board_vendor"), read("/sys/class/dmi/id/board_name")
}
