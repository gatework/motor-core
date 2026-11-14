package migration_test

import (
	"testing"

	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk/contract/migration"
)

// TestPackageLevelForAppReachesTheHostsRegistry is F9's actual scenario: an
// application's init(), reached through nothing but the package-level
// facade, ends up visible in the same Snapshot the host's execution engine
// reads. See the counterproof recorded in PRD 006's dev report: removing
// ForApp/SetVersion/Snapshot at package scope makes the init() above fail
// to compile, which is exactly today's state without this file's
// production counterpart.
func TestPackageLevelForAppReachesTheHostsRegistry(t *testing.T) {

	migration.ForApp("probeapp").SetVersion("1786900000000", func(db *gorm.DB, version, appCode string) error {
		return nil
	})

	entries := migration.Snapshot()
	entry, ok := entries["probeapp-1786900000000"]
	if !ok {
		t.Fatalf("the default registry does not contain what init() registered through the package-level ForApp; got %v", entries)
	}
	if entry.AppCode != "probeapp" {
		t.Errorf("Entry.AppCode = %q, want probeapp", entry.AppCode)
	}
}

// TestPackageLevelSetVersionRegistersUnderTheFrameworkCode covers the other
// package-level entry point: the framework's own migrations
// (go-admin/cmd/migrate/migration/version/*.go) call SetVersion directly
// rather than going through ForApp.
func TestPackageLevelSetVersionRegistersUnderTheFrameworkCode(t *testing.T) {
	migration.SetVersion("1786700009500", func(db *gorm.DB, version string) error { return nil })

	entries := migration.Snapshot()
	entry, ok := entries["1786700009500"]
	if !ok {
		t.Fatalf("no entry for 1786700009500; got %v", entries)
	}
	if entry.AppCode != "" {
		t.Errorf("Entry.AppCode = %q, want empty (framework)", entry.AppCode)
	}
}
