package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTargetMotor(t *testing.T) {
	const old = "github.com/go-admin-team/go-admin-core"
	tests := []struct {
		input string
		want  string
	}{
		{input: old, want: motorPath},
		{input: old + "/v2", want: motorPath},
		{input: old + "/sdk/runtime", want: motorPath + "/sdk/runtime"},
		{input: old + "/v2/sdk/runtime", want: motorPath + "/sdk/runtime"},
		{input: old + "/sdk/pkg/response", want: motorPath + "/response"},
		{input: old + "/v2/sdk/pkg/response", want: motorPath + "/response"},
		{input: old + "-extra/sdk/runtime", want: old + "-extra/sdk/runtime"},
		{input: old + "/v3/sdk/runtime", want: old + "/v3/sdk/runtime"},
		{input: motorPath + "/sdk/runtime", want: motorPath + "/sdk/runtime"},
		{input: "gorm.io/gorm", want: "gorm.io/gorm"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, _, changed := target(tt.input, motorPaths)
			if got != tt.want || changed != (tt.input != tt.want) {
				t.Fatalf("target(%q) = %q, changed=%v; want %q", tt.input, got, changed, tt.want)
			}
		})
	}
}

func TestRewriteMotorPreservesNamesAndIsIdempotent(t *testing.T) {
	src := []byte(`package p

import (
	"github.com/go-admin-team/go-admin-core/tools/gorm/logger"
	jwt "github.com/go-admin-team/go-admin-core/v2/jwtauth"
	_ "github.com/go-admin-team/go-admin-core/v2/sdk"
)

const path = "github.com/go-admin-team/go-admin-core/v2/sdk"
var _ = logger.New
var _ = jwt.New
`)
	got, changes, err := rewrite("x.go", src, motorPaths)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 3 {
		t.Fatalf("rewrote %d imports, want 3", len(changes))
	}
	for _, want := range []string{
		`logger "github.com/gatework/motor-core/tools/gorm/gormlog"`,
		`jwt "github.com/gatework/motor-core/jwtauth"`,
		`_ "github.com/gatework/motor-core/sdk"`,
		`const path = "github.com/go-admin-team/go-admin-core/v2/sdk"`,
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("missing %s in:\n%s", want, got)
		}
	}
	again, changes, err := rewrite("x.go", got, motorPaths)
	if err != nil || len(changes) != 0 || !bytes.Equal(got, again) {
		t.Fatalf("second migration changed its own output: changes=%v, err=%v", changes, err)
	}
}

func TestRunDoesNotFollowSymlinkFiles(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "external.go")
	original := []byte(`package p
import "github.com/go-admin-team/go-admin-core/v2/logger"
`)
	if err := os.WriteFile(target, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "linked.go")); err != nil {
		t.Skip(err)
	}
	if _, _, err := run(dir, true, motorPaths, io.Discard); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatal("rewrote a file outside the migration tree")
	}
}
