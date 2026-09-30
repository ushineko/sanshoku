package sanshoku_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku/internal/apidoc"
)

/*
docs/api.md is generated from the doc comments (spec 008 R2), the way
docs/devices.md is generated from the support table. These tests regenerate it
in memory, so make test finds a stale page before CI's make check-api does. The
fix for either failure is to write the comment and run make generate, never to
relax the test.
*/

// TestTheAPIReferenceIsCurrent fails when docs/api.md and the code disagree,
// naming the first line that differs.
func TestTheAPIReferenceIsCurrent(t *testing.T) {
	want, err := apidoc.Generate(".")
	require.NoError(t, err)
	got, err := os.ReadFile(filepath.Join("docs", "api.md"))
	require.NoError(t, err, "docs/api.md is missing: run make generate")
	if string(got) == want {
		return
	}

	committed := strings.Split(string(got), "\n")
	generated := strings.Split(want, "\n")
	for i := 0; i < max(len(committed), len(generated)); i++ {
		c, g := lineAt(committed, i), lineAt(generated, i)
		if c != g {
			t.Fatalf("docs/api.md is stale at line %d: run make generate\n  committed: %s\n  generated: %s",
				i+1, c, g)
		}
	}
}

// lineAt is line i, or a marker past the end.
func lineAt(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return "(end of file)"
}

// TestEveryExportedIdentifierIsDocumented fails on an exported identifier in a
// public package with no doc comment, naming the package and the identifier.
// Methods count, including ones that only implement an interface.
func TestEveryExportedIdentifierIsDocumented(t *testing.T) {
	missing, err := apidoc.Undocumented(".")
	require.NoError(t, err)
	for _, m := range missing {
		t.Errorf("%s is exported and has no doc comment", m)
	}
}
