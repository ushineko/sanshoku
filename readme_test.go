package sanshoku_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

/*
The README is the module's front page and nothing builds it. These canaries
fail when the repository and the page have drifted apart (spec 008 R1), and the
fix is always to write the missing line, never to relax the test. fynedesygn's
README drifted twenty-four releases before a test read it.
*/

// module is the import path the README's reference links are built from.
const module = "github.com/ushineko/sanshoku"

// plumbingTargets are the make targets the Development block deliberately
// does not list.
var plumbingTargets = map[string]bool{
	"help":         true, // prints the list this test reads
	"install-lint": true, // a dependency of lint and setup
	"clean":        true, // does what it says
}

// readme is the front page, read once per test.
func readme(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("README.md")
	require.NoError(t, err)
	return string(b)
}

// section is the part of the README between two headings, so a name is looked
// for where it belongs: a package named in the changelog is not in the table.
func section(t *testing.T, doc, from, to string) string {
	t.Helper()
	i := strings.Index(doc, from)
	require.Positive(t, i, "the README has no %q heading", from)
	rest := doc[i+len(from):]
	j := strings.Index(rest, to)
	require.Positive(t, j, "the README has no %q heading after %q", to, from)
	return rest[:j]
}

// TestEveryPackageIsInTheReadme fails when a package or a command has no row
// in the "What is in it" table, or has a row with no link to its reference.
func TestEveryPackageIsInTheReadme(t *testing.T) {
	table := section(t, readme(t), "## What is in it", "## What it does not do")
	for _, dir := range goPackageDirs(t) {
		name, link := dir, module+"/"+dir
		if dir == "." {
			name, link = "sanshoku", module
		}
		if !strings.Contains(table, "[`"+name+"`](https://pkg.go.dev/"+link+")") {
			t.Errorf("package %s has no row in the What is in it table", name)
		}
	}
}

// goPackageDirs lists every directory holding non-test Go source, relative to
// the module root, as go list ./... would: hidden directories and testdata are
// skipped.
func goPackageDirs(t *testing.T) []string {
	t.Helper()
	var dirs []string
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		base := d.Name()
		if path != "." && (strings.HasPrefix(base, ".") || strings.HasPrefix(base, "_") || base == "testdata") {
			return filepath.SkipDir
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if n := e.Name(); strings.HasSuffix(n, ".go") && !strings.HasSuffix(n, "_test.go") {
				dirs = append(dirs, filepath.ToSlash(path))
				break
			}
		}
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, dirs)
	return dirs
}

// TestEveryMakeTargetIsInTheReadme fails when a make target with a ##
// description has no `make <target>` line in the Development block.
func TestEveryMakeTargetIsInTheReadme(t *testing.T) {
	dev := section(t, readme(t), "## Development", "## Licence")
	b, err := os.ReadFile("Makefile")
	require.NoError(t, err)

	matches := regexp.MustCompile(`(?m)^([a-z][a-z-]*):[^\n]*## `).FindAllStringSubmatch(string(b), -1)
	require.NotEmpty(t, matches)
	for _, m := range matches {
		target := m[1]
		if plumbingTargets[target] {
			continue
		}
		require.Regexp(t, `(?m)^make `+regexp.QuoteMeta(target)+`\b`, dev,
			"make %s is in the Makefile but not in the Development block", target)
	}
}

// TestEveryDocumentIsLinkedFromTheReadme fails when a page in docs/ is not
// linked from the Documentation list.
func TestEveryDocumentIsLinkedFromTheReadme(t *testing.T) {
	links := section(t, readme(t), "## Documentation", "## Development")
	files, err := filepath.Glob(filepath.Join("docs", "*.md"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, f := range files {
		link := filepath.ToSlash(f)
		if !strings.Contains(links, "("+link+")") {
			t.Errorf("%s is not linked from the Documentation list", link)
		}
	}
}

// TestTheVersionLineMatchesTheChangelog fails when the Version line is not the
// newest released changelog heading, or when an Unreleased heading sits below
// a released one. It cannot compare with the latest tag: a CI checkout has no
// tags, and the rule that they match is applied when tagging.
func TestTheVersionLineMatchesTheChangelog(t *testing.T) {
	doc := readme(t)
	version := regexp.MustCompile(`(?m)^\*\*Version\*\*: (\d+\.\d+\.\d+)$`).FindStringSubmatch(doc)
	require.Len(t, version, 2, "the README has no **Version** line")

	at := strings.Index(doc, "## Changelog")
	require.Positive(t, at, "the README has no changelog")
	changelog := doc[at:]

	released := regexp.MustCompile(`(?m)^### (\d+\.\d+\.\d+) \(\d{4}-\d{2}-\d{2}\)$`).FindStringSubmatchIndex(changelog)
	require.NotNil(t, released, "the changelog has no released heading")
	newest := changelog[released[2]:released[3]]
	require.Equal(t, newest, version[1],
		"the Version line says %s but the newest changelog heading is %s", version[1], newest)

	if unreleased := strings.Index(changelog, "\n### Unreleased\n"); unreleased >= 0 {
		require.Less(t, unreleased, released[0],
			"### Unreleased is below ### %s; it belongs above the newest release", newest)
	}
}

// TestTheReadmeExampleCompiles fails when the README's "Using it" block is not
// the body of Example in example_test.go, character for character. The
// Example is compiled by go test, so the block is code that builds.
func TestTheReadmeExampleCompiles(t *testing.T) {
	using := section(t, readme(t), "## Using it", "## Devices")
	block := regexp.MustCompile("(?s)```go\n(.*?)```").FindStringSubmatch(using)
	require.Len(t, block, 2, "the Using it section has no ```go block")

	require.Equal(t, exampleBody(t), block[1],
		"the README's Using it block is not Example's body in example_test.go: copy the body across")
}

// exampleBody is the source between Example's braces, one tab of indentation
// removed, as it would be pasted into the README.
func exampleBody(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile("example_test.go")
	require.NoError(t, err)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "example_test.go", src, parser.ParseComments)
	require.NoError(t, err)

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "Example" || fn.Body == nil {
			continue
		}
		open := fset.Position(fn.Body.Lbrace).Offset
		end := fset.Position(fn.Body.Rbrace).Offset
		body := strings.TrimPrefix(string(src[open+1:end]), "\n")
		lines := strings.Split(body, "\n")
		for i, l := range lines {
			lines[i] = strings.TrimPrefix(l, "\t")
		}
		return strings.Join(lines, "\n")
	}
	t.Fatal("example_test.go has no func Example")
	return ""
}
