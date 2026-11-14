// Command coreupgrade rewrites imports of the packages this module deprecated
// in v1.6.0 to the paths that replace them in v2.0.0.
//
// Usage:
//
//	go run github.com/gatework/motor-core/tools/coreupgrade@latest -motor [-w] DIR
//
// It reports what it would change and exits. Pass -w to write the files.
package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func main() {
	write := flag.Bool("w", false, "write the files instead of reporting what would change")
	major := flag.Bool("v2", false, "also move every import of this module onto its v2 path")
	motor := flag.Bool("motor", false, "migrate legacy and v2 imports to github.com/gatework/motor-core")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: coreupgrade [-w] [-v2 | -motor] DIR\n\n")
		fmt.Fprintf(os.Stderr, "Rewrites deprecated go-admin-core imports. Without -w it only reports.\n")
		fmt.Fprintf(os.Stderr, "With -v2 it also moves every import of this module onto the v2 path,\n")
		fmt.Fprintf(os.Stderr, "which Go requires from that major version on. Run go mod tidy after.\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	if *major && *motor {
		fmt.Fprintln(os.Stderr, "coreupgrade: -v2 and -motor are mutually exclusive")
		os.Exit(2)
	}
	dest := legacyPaths
	if *major {
		dest = v2Paths
	}
	if *motor {
		dest = motorPaths
	}
	n, files, err := run(flag.Arg(0), *write, dest, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "coreupgrade:", err)
		os.Exit(1)
	}

	switch {
	case n == 0:
		fmt.Println("no deprecated imports found")
	case *write:
		fmt.Printf("\nrewrote %d import(s) in %d file(s)\n", n, files)
	default:
		fmt.Printf("\n%d import(s) in %d file(s) would be rewritten; pass -w to apply\n", n, files)
	}
}

type migration struct {
	root           *os.Root
	write          bool
	dest           destination
	out            io.Writer
	imports, files int
}

func run(dir string, write bool, dest destination, out io.Writer) (int, int, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return 0, 0, err
	}
	defer root.Close()
	m := &migration{root: root, write: write, dest: dest, out: out}
	err = fs.WalkDir(root.FS(), ".", m.visit)
	return m.imports, m.files, err
}

func (m *migration) visit(name string, entry fs.DirEntry, walkErr error) error {
	if walkErr != nil {
		return walkErr
	}
	if entry.IsDir() {
		if name != "." && skipDir(entry.Name()) {
			return fs.SkipDir
		}
		return nil
	}
	if !entry.Type().IsRegular() || !strings.HasSuffix(name, ".go") {
		return nil
	}
	source, err := m.root.ReadFile(name)
	if err != nil {
		return err
	}
	next, changes, err := rewrite(name, source, m.dest)
	if err != nil {
		// Parse failures are reported without deciding the consumer's build policy.
		fmt.Fprintln(os.Stderr, "coreupgrade: skipping", err)
		return nil
	}
	if len(changes) == 0 {
		return nil
	}
	m.report(name, changes)
	if m.write {
		if err := m.writeFile(name, next); err != nil {
			return err
		}
	}
	m.imports += len(changes)
	m.files++
	return nil
}

func (m *migration) report(name string, changes []change) {
	for _, change := range changes {
		note := ""
		if change.Aliased {
			note = fmt.Sprintf(" (kept the local name %q; the package is now %q)", packageName(change.From), packageName(change.To))
		}
		fmt.Fprintf(m.out, "%s:%d: %s -> %s%s\n", name, change.Line, change.From, change.To, note)
	}
}

func (m *migration) writeFile(name string, content []byte) error {
	info, err := m.root.Stat(name)
	if err != nil {
		return err
	}
	// Both reads and replacement stay within the selected consumer directory.
	// A symlink swap cannot redirect the migration outside that root.
	temporary := filepath.Join(filepath.Dir(name), ".coreupgrade-"+rand.Text())
	file, err := m.root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = m.root.Remove(temporary) }()
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Chmod(info.Mode().Perm()); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return m.root.Rename(temporary, name)
}

func skipDir(name string) bool {
	switch name {
	case ".git", "vendor", "node_modules", "testdata":
		return true
	}
	return false
}

// packageName is asked about import paths, which are slash separated on every
// platform, so it uses path rather than filepath.
func packageName(importPath string) string {
	for _, m := range moves {
		switch importPath {
		case m.from:
			return m.name
		case m.to:
			return m.newName
		}
	}
	return path.Base(importPath)
}
