package runtime

import (
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
)

// The GetAll accessors hand out the map itself. Three of them take the lock
// while doing so, which protects the return statement and nothing else: the
// caller reads the map long after the lock is gone. The other two do not lock
// at all. Either way a setter running alongside a caller is a data race, and
// an iteration alongside a write is a fatal error rather than a failed test.
func TestGetAllDBIsSafeAgainstAWriter(t *testing.T) {
	e := New()

	stop := make(chan struct{})
	var wg sync.WaitGroup

	wg.Go(func() {
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			e.SetDB(string(rune('a'+i%26)), &gorm.DB{})
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			for range e.Databases() {
			}
		}
	})

	time.Sleep(200 * time.Millisecond)
	close(stop)
	wg.Wait()
}
