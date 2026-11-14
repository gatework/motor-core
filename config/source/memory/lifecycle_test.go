package memory

import (
	"fmt"
	"github.com/gatework/motor-core/config/source"
	"testing"
	"time"
)

func TestWatcherStopUnblocksNext(t *testing.T) {
	s := NewSource(WithJSON([]byte(`{}`)))
	w, err := s.Watch()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := w.Next(); done <- err }()
	if err := w.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := w.Stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("stopped watcher returned no error")
		}
	case <-time.After(time.Second):
		t.Fatal("Next remained blocked after Stop")
	}
}
func TestSourceOwnsData(t *testing.T) {
	data := []byte(`{"key":1}`)
	s := NewSource(WithJSON(data))
	data[0] = '!'
	first, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if first.Data[0] != '{' {
		t.Fatal("source retained caller data")
	}
	first.Data[0] = '!'
	next, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if next.Data[0] != '{' {
		t.Fatal("Read exposed source data")
	}
	w, err := s.Watch()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	if err := s.Write(&source.ChangeSet{Data: []byte(`{"key":2}`), Format: "json"}); err != nil {
		t.Fatal(err)
	}
	update, err := w.Next()
	if err != nil {
		t.Fatal(err)
	}
	update.Data[0] = '!'
	next, err = s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if next.Data[0] != '{' {
		t.Fatal("Next exposed source data")
	}
}

func TestSlowWatcherReceivesLatestState(t *testing.T) {
	s := NewSource(WithJSON([]byte(`{}`)))
	w, err := s.Watch()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	for i := 0; i < 200; i++ {
		if err := s.Write(&source.ChangeSet{Data: []byte(fmt.Sprintf(`{"key":%d}`, i)), Format: "json"}); err != nil {
			t.Fatal(err)
		}
	}
	latest, err := w.Next()
	if err != nil {
		t.Fatal(err)
	}
	if string(latest.Data) != `{"key":199}` {
		t.Fatalf("slow watcher received stale state: %s", latest.Data)
	}
}
