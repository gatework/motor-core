package runtime

import (
	"net/http"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

// Acceptance 12: the accessors that used to touch their fields with no lock at
// all - before, appRouters and engine - hammered from several goroutines at
// once. Under -race this fails on the old code and says nothing on the new.
//
// engine is the one that mattered in production: Routers reads it at run
// time, from the manual "sync API" endpoint, while SetEngine wrote it with no
// lock on the other side.
func TestUnguardedAccessorsAreRaceFree(t *testing.T) {
	gin.SetMode(gin.TestMode)
	captureLogs(t, New())

	app := New()
	engines := []http.Handler{gin.New(), gin.New()}
	for _, ge := range engines {
		ge.(*gin.Engine).GET("/ping", func(c *gin.Context) {})
	}
	app.SetEngine(engines[0])

	const goroutines, iters = 16, 100
	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := range iters {
				switch (i + j) % 8 {
				case 0:
					app.SetBefore(func() {})
				case 1:
					app.BeforeSealed()
				case 2:
					app.SetAppRouters(func() {})
				case 3:
					app.AppRoutersSealed()
				case 4:
					app.SetEngine(engines[j%len(engines)])
				case 5:
					app.Engine()
				case 6:
					app.Routers()
				case 7:
					app.BeforeSealed()
					app.AppRoutersSealed()
				}
			}
		}(i)
	}
	wg.Wait()
}

// Running a registry while other goroutines are still registering into it.
//
// The seal, the cursor and the batch of callbacks to run all move inside one
// critical section, which is what leaves no middle state: a registration
// either lands in the batch or is refused as late. What must never happen is a
// callback that is accepted and then never run.
func TestConcurrentRunAndRegisterIsRaceFree(t *testing.T) {
	app := New()
	captureLogs(t, app)

	var mu sync.Mutex
	ran := 0

	const goroutines = 16
	var wg sync.WaitGroup
	for range goroutines {
		wg.Go(func() {
			app.SetBefore(func() {
				mu.Lock()
				ran++
				mu.Unlock()
			})
		})
	}
	for range 4 {
		wg.Go(func() {
			app.RunBefore()
		})
	}
	wg.Wait()

	// Whatever the interleaving was, the registry is closed and everything it
	// accepted has run exactly once.
	app.RunBefore()
	if !app.BeforeSealed() {
		t.Fatal("the registry is not closed after RunBefore")
	}

	accepted := len(app.before.entries)
	mu.Lock()
	defer mu.Unlock()
	if ran != accepted {
		t.Errorf("%d callbacks were accepted but %d ran; an accepted registration must never be dropped", accepted, ran)
	}
}
