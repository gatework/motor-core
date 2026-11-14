package runtime

import (
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

// Cloning the map is not enough when the values are slices. The hazard is not
// the accessor racing AddHandler — three seconds of that reports
// nothing, because an append writes at index len, outside the window a reader
// holds. It is a caller appending to what it was handed: that writes the same
// slot the next AddHandler writes, and only when the internal slice
// has spare capacity, which is why this sets three handlers rather than four.
func TestCallersAppendingToTheSnapshotDoNotShare(t *testing.T) {
	e := New()
	noop := func(r *gin.RouterGroup, hand ...*gin.HandlerFunc) {}
	for i := 0; i < 3; i++ {
		e.AddHandler("t", noop)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				for _, fns := range e.AllHandlers() {
					_ = append(fns, noop)
				}
			}
		}()
	}
	wg.Wait()
}
