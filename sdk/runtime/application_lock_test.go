package runtime

import (
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// sync.RWMutex is not reentrant: taking the read lock while already holding
// the write lock deadlocks the goroutine. Every accessor below runs inside a
// locked section, so none of them may call DefaultTenant.
//
// These calls used to hang forever. They were never exercised by any consumer,
// which is why the defect survived unnoticed — hence this guard.

func mustReturn(t *testing.T, name string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Errorf("%s did not return within 2s: lock reentrancy deadlock", name)
	}
}

func TestSingleTenantAccessorsDoNotDeadlock(t *testing.T) {
	cases := []struct {
		name string
		fn   func()
	}{
		{"SetDB", func() { New().SetDB(DefaultTenant, &gorm.DB{}) }},
		{"SetApp", func() { New().SetApp(DefaultTenant, "app") }},
		{"SetCasbin", func() { New().SetCasbin(DefaultTenant, nil) }},
		{"SetCasbinExclude", func() { New().SetCasbinExclude(DefaultTenant, nil) }},
		{"SetCrontab", func() { New().SetCrontab(DefaultTenant, nil) }},
		{"SetHandler", func() { New().AddHandler(DefaultTenant, nil) }},
		{"GetHandler", func() { New().Handlers(DefaultTenant) }},
		{"GetConfig", func() { New().Config(DefaultTenant) }},
		{"GetDB", func() { New().DB(DefaultTenant) }},
		{"GetCasbin", func() { New().Casbin(DefaultTenant) }},
		{"GetCrontab", func() { New().Crontab(DefaultTenant) }},
		{"GetConfigValue", func() { New().ConfigValue(DefaultTenant, "k") }},
		{"SetConfigValue", func() { New().SetConfigValue(DefaultTenant, "k", "v") }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mustReturn(t, c.name, c.fn)
		})
	}
}

// The single-tenant setters must write under the default tenant key, i.e. be
// equivalent to their explicit *ByTenant counterparts.
func TestSingleTenantSettersUseDefaultTenant(t *testing.T) {
	app := New()
	db := &gorm.DB{}
	app.SetDB(DefaultTenant, db)

	if got := app.DB(DefaultTenant); got != db {
		t.Errorf("SetDB did not store under %q", DefaultTenant)
	}

	app.SetConfigValue(DefaultTenant, "key", "value")
	if got := app.ConfigValue(DefaultTenant, "key"); got != "value" {
		t.Errorf("SetConfigValue did not store under %q, got %v", DefaultTenant, got)
	}
}

// Acceptance 14: the same guard applied to everything this batch put behind
// the mutex. The failure mode is identical - a method that takes the lock and
// then calls another one that takes it hangs forever - and it is invisible
// until the one call order that triggers it happens in production.
func TestRegistryAccessorsDoNotDeadlock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	noop := func() {}

	cases := []struct {
		name string
		fn   func()
	}{
		{"SetBefore", func() { New().SetBefore(noop) }},
		{"SetBeforeWith", func() { New().SetBeforeWith(noop, WithName("x"), WithFatal()) }},
		{"RunBefore", func() { New().RunBefore() }},
		{"BeforeSealed", func() { New().BeforeSealed() }},
		{"SetAppRouters", func() { New().SetAppRouters(noop) }},
		{"SetAppRoutersWith", func() { New().SetAppRoutersWith(noop, WithName("x")) }},
		{"RunAppRouters", func() { New().RunAppRouters() }},
		{"AppRoutersSealed", func() { New().AppRoutersSealed() }},
		{"SetEngine", func() { New().SetEngine(gin.New()) }},
		{"Engine", func() { New().Engine() }},
		{"Routers", func() { New().Routers() }},
		{"Routers with a gin engine", func() {
			app := New()
			app.SetEngine(gin.New())
			app.Routers()
		}},
		// The one that is not hypothetical: app/demo/router/router.go reads and
		// writes the engine from inside the very callback RunAppRouters
		// executes. Running callbacks under the lock hangs right here.
		{"RunAppRouters with a callback that takes the lock", func() {
			app := New()
			app.SetAppRouters(func() {
				if app.Engine() == nil {
					app.SetEngine(gin.New())
				}
			})
			app.RunAppRouters()
		}},
		{"RunBefore with a callback that takes the lock", func() {
			app := New()
			app.SetBefore(func() { app.SetConfigValue(DefaultTenant, "k", "v") })
			app.RunBefore()
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mustReturn(t, c.name, c.fn)
		})
	}
}
