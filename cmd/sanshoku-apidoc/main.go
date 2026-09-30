/*
sanshoku-apidoc prints docs/api.md, the signature index of the module's public
packages, generated from their doc comments. Run from the module root; `make
generate` writes the page and `make check-api` fails when the committed page
differs. Spec 008 R2.1.
*/
package main

import (
	"fmt"
	"os"

	"github.com/ushineko/sanshoku/internal/apidoc"
)

func main() {
	page, err := apidoc.Generate(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "sanshoku-apidoc:", err)
		os.Exit(1)
	}
	fmt.Print(page)
}
