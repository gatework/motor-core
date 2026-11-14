package loader

import (
	"testing"

	"github.com/gatework/motor-core/config/source"
)

func TestCopyOwnsChangeSetData(t *testing.T) {
	original := &Snapshot{ChangeSet: &source.ChangeSet{Data: []byte(`{}`)}, Version: "1"}
	cloned := Copy(original)
	cloned.ChangeSet.Data[0] = '!'
	if original.ChangeSet.Data[0] != '{' {
		t.Fatal("Copy retained snapshot byte slice")
	}
}
