package main

import (
	"bytes"
	"cmp"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"strings"
)

// change is one import line the tool would replace, reported whether or not
// the file is written.
type change struct {
	Line    int
	From    string
	To      string
	Aliased bool
}

// rewrite returns src with every deprecated import replaced. It parses rather
// than substituting text so that the path appearing in a comment, a string
// literal or a struct tag is left alone; only the import block is touched, and
// the rest of the file keeps its formatting byte for byte.
func rewrite(filename string, src []byte, dest destination) ([]byte, []change, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, parser.ImportsOnly|parser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", filename, err)
	}

	type edit struct {
		start, end int
		text       string
	}

	var edits []edit
	var changes []change

	for _, spec := range f.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}

		to, m, ok := target(path, dest)
		if !ok {
			continue
		}

		// The local name has to survive the move. An alias or a dot import
		// already fixes it, so only a plain import of a renamed package needs
		// one added — and the name it needs is free by construction, because
		// the import being replaced was using it.
		added := spec.Name == nil && m.name != m.newName
		name := ""
		switch {
		case spec.Name != nil:
			name = spec.Name.Name
		case added:
			name = m.name
		}

		text := strconv.Quote(to)
		if name != "" {
			text = name + " " + text
		}

		edits = append(edits, edit{
			start: fset.Position(spec.Pos()).Offset,
			end:   fset.Position(spec.End()).Offset,
			text:  text,
		})
		changes = append(changes, change{
			Line:    fset.Position(spec.Pos()).Line,
			From:    path,
			To:      to,
			Aliased: added,
		})
	}

	if len(edits) == 0 {
		return src, nil, nil
	}

	// Applied back to front so that earlier offsets stay valid.
	slices.SortFunc(edits, func(a, b edit) int { return cmp.Compare(b.start, a.start) })
	out := src
	for _, e := range edits {
		merged := make([]byte, 0, len(out)+len(e.text))
		merged = append(merged, out[:e.start]...)
		merged = append(merged, e.text...)
		merged = append(merged, out[e.end:]...)
		out = merged
	}

	// A path that changes usually sorts somewhere else, and leaving it where
	// the old one sat makes the block unsorted. gofmt sorts each group, so the
	// result is formatted — but only if the input was, because reformatting a
	// file the tool did not have to touch buries the migration in noise.
	if formatted, err := format.Source(out); err == nil {
		if clean, err := format.Source(src); err == nil && bytes.Equal(clean, src) {
			out = formatted
		}
	}

	return out, changes, nil
}

// target returns the path that replaces importPath. The two rules compose in
// one direction only: a package is moved first and then carried across the
// major version, so sdk/pkg/response becomes response becomes v2/response.
func target(importPath string, dest destination) (string, move, bool) {
	if dest == motorPaths {
		return motorTarget(importPath)
	}
	m, moved := lookup(importPath)

	out := importPath
	if moved {
		out = m.to
	}
	if dest == v2Paths {
		if major, ok := withMajor(out); ok {
			out = major
		}
	}
	return out, m, out != importPath
}

// withMajor puts a path of this module on its major version. A path already
// there is left alone, and so is a module whose path merely starts with the
// same characters — go-admin-core-extra is not this module.
func withMajor(importPath string) (string, bool) {
	switch {
	case importPath == majorPath || strings.HasPrefix(importPath, majorPath+"/"):
		return "", false
	case importPath == strings.TrimSuffix(mod, "/"):
		return majorPath, true
	case strings.HasPrefix(importPath, mod):
		return majorPath + "/" + strings.TrimPrefix(importPath, mod), true
	}
	return "", false
}

func lookup(path string) (move, bool) {
	for _, m := range moves {
		if m.from == path {
			return m, true
		}
	}
	return move{}, false
}

// motorTarget accepts only the old module's unversioned and v2 paths.
// A different module or an unsupported major version must not be rewritten.
func motorTarget(importPath string) (string, move, bool) {
	bare := strings.TrimSuffix(mod, "/")
	suffix := ""
	switch {
	case importPath == majorPath, importPath == bare:
	case strings.HasPrefix(importPath, majorPath+"/"):
		suffix = strings.TrimPrefix(importPath, majorPath+"/")
	case strings.HasPrefix(importPath, mod):
		suffix = strings.TrimPrefix(importPath, mod)
		first, _, _ := strings.Cut(suffix, "/")
		if len(first) > 1 && first[0] == 'v' {
			if _, err := strconv.Atoi(first[1:]); err == nil {
				return importPath, move{}, false
			}
		}
	default:
		return importPath, move{}, false
	}
	m, moved := lookup(mod + suffix)
	if moved {
		suffix = strings.TrimPrefix(m.to, mod)
	}
	out := motorPath
	if suffix != "" {
		out += "/" + suffix
	}
	return out, m, true
}
