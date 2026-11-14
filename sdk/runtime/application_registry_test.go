package runtime

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/storage"
)

// recordingLogger keeps every line so a test can assert on what was said.
//
// It reports TraceLevel so nothing is filtered out: the guard's own defence
// against Helper.Fatalf is that level filtering must never decide whether
// something is reported, and a test logger that filters would hide exactly the
// regression this file exists to catch.
type recordingLogger struct {
	mu    sync.Mutex
	lines []string
}

func (l *recordingLogger) With(...logger.Field) logger.Logger { return l }
func (l *recordingLogger) Enabled(logger.Level) bool          { return true }
func (l *recordingLogger) Log(_ context.Context, level logger.Level, msg string, _ ...logger.Field) {
	l.record(level, msg)
}

func (l *recordingLogger) record(level logger.Level, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, level.String()+" "+msg)
}

func (l *recordingLogger) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.lines...)
}

// only returns the lines logged at one level.
func (l *recordingLogger) only(level logger.Level) []string {
	prefix := level.String() + " "
	var out []string
	for _, line := range l.all() {
		if strings.HasPrefix(line, prefix) {
			out = append(out, line)
		}
	}
	return out
}

// captureLogs points the application at a recording logger for one test.
//
// Logger ownership is local to this application.
func captureLogs(t *testing.T, app *Application) *recordingLogger {
	t.Helper()
	rec := &recordingLogger{}
	previous := app.Logger()
	t.Cleanup(func() { app.SetLogger(previous) })
	app.SetLogger(rec)
	return rec
}

// Acceptance 1: RunBefore runs every registered callback, in the order they
// were registered. The order is part of the contract - it is what lets a
// module rely on an earlier one having run - so it is asserted, not assumed.
func TestRunBeforeRunsInRegistrationOrder(t *testing.T) {
	app := New()
	var order []string
	for _, name := range []string{"first", "second", "third"} {
		app.SetBefore(func() { order = append(order, name) })
	}

	app.RunBefore()

	if got := strings.Join(order, ","); got != "first,second,third" {
		t.Errorf("ran %q, want first,second,third", got)
	}
}

// Acceptance 2: the same promise for the router registry.
func TestRunAppRoutersRunsInRegistrationOrder(t *testing.T) {
	app := New()
	var order []string
	for _, name := range []string{"first", "second", "third"} {
		app.SetAppRouters(func() { order = append(order, name) })
	}

	app.RunAppRouters()

	if got := strings.Join(order, ","); got != "first,second,third" {
		t.Errorf("ran %q, want first,second,third", got)
	}
}

// Acceptance 3: the pre-existing way of using the registry - ask for the
// callbacks and run them yourself - keeps working unchanged. Consumers on that
// pattern are not required to move to Run*().

func TestRunIsIdempotent(t *testing.T) {
	app := New()
	before, routers := 0, 0
	app.SetBefore(func() { before++ })
	app.SetAppRouters(func() { routers++ })

	for range 3 {
		app.RunBefore()
		app.RunAppRouters()
	}

	if before != 1 || routers != 1 {
		t.Errorf("before ran %d times and appRouters %d, want 1 each", before, routers)
	}
}

// Acceptance 9: a registration that arrives after the registry has run is
// dropped and said out loud. Keeping it would be worse: it would sit in a
// slice nobody walks again, which is the silent failure this batch exists to
// remove.
func TestRegistrationAfterRunIsDroppedAndReported(t *testing.T) {
	app := New()
	rec := captureLogs(t, app)

	ran := 0
	app.SetBefore(func() { ran++ })
	app.RunBefore()

	late := 0
	app.SetBefore(func() { late++ })
	app.RunBefore()

	if late != 0 {
		t.Errorf("the late callback ran %d times, want 0", late)
	}
	if ran != 1 {
		t.Errorf("the early callback ran %d times, want 1", ran)
	}
	if got := len(app.before.entries); got != 1 {
		t.Errorf("the registry holds %d callbacks, want 1", got)
	}

	errors := rec.only(logger.ErrorLevel)
	if len(errors) != 1 {
		t.Fatalf("want exactly one error line, got %v", errors)
	}
	// What happened, why, and where the offending registration is: a line
	// missing any of the three leaves the reader with nothing to act on.
	for _, want := range []string{"SetBefore", "ignored", "RunBefore", "application_registry_test.go"} {
		if !strings.Contains(errors[0], want) {
			t.Errorf("the error must mention %q, got: %s", want, errors[0])
		}
	}
}

// The registries close one at a time. They have separate entry points, so
// running one says nothing about whether the other is still open.
func TestSealingIsPerRegistry(t *testing.T) {
	app := New()
	captureLogs(t, app)

	app.RunBefore()
	if !app.BeforeSealed() {
		t.Error("RunBefore must close the before registry")
	}
	if app.AppRoutersSealed() {
		t.Error("RunBefore must not close the appRouters registry")
	}

	ran := false
	app.SetAppRouters(func() { ran = true })
	app.RunAppRouters()
	if !ran {
		t.Error("a router registered after RunBefore must still run")
	}
	if !app.AppRoutersSealed() {
		t.Error("RunAppRouters must close the appRouters registry")
	}
}

// labelledCache and labelledQueue carry an id so a test can tell which
// instance the getters hand back; the real memory adapters both report their
// type, which would pass whether or not the field was replaced.
type labelledCache struct {
	storage.Cache
	id string
}

func (c *labelledCache) String() string { return c.id }

type labelledQueue struct {
	storage.Queue
	id string
}

func (q *labelledQueue) String() string { return q.id }

// Acceptance 10: closing the registries must not reach the resource fields.
// Reloading the configuration re-runs the setup callbacks while requests are
// being served - replacing the database, the cache and the queue - so freezing
// those would turn a config change into a silent no-op.
func TestSealingDoesNotFreezeResources(t *testing.T) {
	app := New()
	captureLogs(t, app)

	app.RunBefore()
	app.RunAppRouters()

	db := &gorm.DB{}
	app.SetDB("tenant-a", db)
	if got := app.DB("tenant-a"); got != db {
		t.Error("SetDB stopped working once the registries were closed")
	}

	app.SetCache(&labelledCache{id: "cache-after-seal"})
	if got := app.Cache().(*labelledCache).String(); got != "cache-after-seal" {
		t.Errorf("Cache returned %q, want the adapter set after sealing", got)
	}

	app.SetQueue(&labelledQueue{id: "queue-after-seal"})
	if got := app.Queue().(*labelledQueue).String(); got != "queue-after-seal" {
		t.Errorf("Queue returned %q, want the adapter set after sealing", got)
	}

	app.SetConfigValue(DefaultTenant, "key", "value")
	if got := app.ConfigValue(DefaultTenant, "key"); got != "value" {
		t.Errorf("GetConfigValue returned %v, want the value set after sealing", got)
	}
}

// A callback reaching back into Application is the normal case, not an edge
// one: app/demo/router/router.go asks for the engine and sets it when there is
// none, and that function is exactly what RunAppRouters executes. sync.RWMutex
// is not reentrant, so running callbacks under the lock hangs here - which is
// why the guard is a timeout rather than a plain call.
func TestCallbackMayCallBackIntoRuntime(t *testing.T) {
	app := New()
	gin.SetMode(gin.TestMode)

	var seen http.Handler
	mustReturn(t, "RunAppRouters with a callback that touches Application", func() {
		app.SetAppRouters(func() {
			if h := app.Engine(); h == nil {
				app.SetEngine(gin.New())
			}
			seen = app.Engine()
			app.Routers()
			app.DefaultTenant()
		})
		app.RunAppRouters()
	})

	if seen == nil {
		t.Error("the callback did not observe the engine it had just set")
	}
}

// Mixing the two styles runs the callbacks twice, and core cannot prevent that
// - it never sees a loop written downstream. What it can do is say so, which
// is the difference between a startup that is wrong and one that is wrong and
// undiagnosable.

func TestWithOptionsSharesThePlainPath(t *testing.T) {
	app := New()
	var order []string
	app.SetBefore(func() { order = append(order, "plain") })
	app.SetBeforeWith(func() { order = append(order, "with-name") }, WithName("named"))
	app.SetAppRouters(func() { order = append(order, "router-plain") })
	app.SetAppRoutersWith(func() { order = append(order, "router-named") }, WithName("named"))

	app.RunBefore()
	app.RunAppRouters()

	if got := strings.Join(order, ","); got != "plain,with-name,router-plain,router-named" {
		t.Errorf("ran %q, want plain,with-name,router-plain,router-named", got)
	}
}

// The empty case used to return the nil field itself. Handing back an empty
// non-nil slice instead would be a visible change for anyone who compares
// against nil, and there is nothing to gain from it.
