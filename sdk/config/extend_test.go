package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gatework/motor-core/config/source/file"
)

func resetExtend(t *testing.T) {
	t.Helper()
	isolateSettings(t)
	extendMu.Lock()
	previous := extendTargets
	extendTargets = map[string]extendSlot{}
	extendMu.Unlock()
	current.Store(nil)
	t.Cleanup(func() { extendMu.Lock(); extendTargets = previous; extendMu.Unlock() })
}

type orderExtend struct{ Endpoint string }
type paymentExtend struct {
	Endpoint string
	Timeout  int
}

func TestRegisterExtendKeepsSectionsIndependent(t *testing.T) {
	resetExtend(t)
	order := RegisterExtend[orderExtend]("order")
	payment := RegisterExtend[paymentExtend]("payment")
	decodeExtend(t, `{"extend":{"order":{"Endpoint":"orders"},"payment":{"Endpoint":"payments","Timeout":30}}}`)
	if order().Endpoint != "orders" || payment().Endpoint != "payments" || payment().Timeout != 30 {
		t.Fatal("registered sections were not independent")
	}
	first := order()
	decodeExtend(t, `{"extend":{"order":{"Endpoint":"new-orders"}}}`)
	if first.Endpoint != "orders" || order().Endpoint != "new-orders" || payment().Endpoint != "payments" {
		t.Fatal("reload mutated an old snapshot or removed a missing section")
	}
}

func TestRegisterExtendPanicsOnDuplicateKey(t *testing.T) {
	resetExtend(t)
	RegisterExtend[orderExtend]("order")
	defer func() {
		message := fmt.Sprint(recover())
		if !strings.Contains(message, "order") || !strings.Contains(message, "twice") {
			t.Fatalf("unexpected panic: %s", message)
		}
	}()
	RegisterExtend[paymentExtend]("order")
}

func TestRegisterExtendPanicsOnEmptyKey(t *testing.T) {
	resetExtend(t)
	defer func() {
		if recover() == nil {
			t.Error("empty key did not panic")
		}
	}()
	RegisterExtend[orderExtend]("")
}

func TestRegisterExtendAccessorNeverReturnsNil(t *testing.T) {
	resetExtend(t)
	if RegisterExtend[orderExtend]("order")() == nil {
		t.Fatal("accessor returned nil")
	}
}

func TestExtendDispatcherNamesTheFailingKey(t *testing.T) {
	resetExtend(t)
	order := RegisterExtend[orderExtend]("order")
	RegisterExtend[paymentExtend]("payment")
	decodeExtend(t, `{"extend":{"order":{"Endpoint":"first"},"payment":{"Endpoint":"first"}}}`)
	_, err := prepareExtensions(map[string]json.RawMessage{"order": json.RawMessage(`{"Endpoint":"second"}`), "payment": json.RawMessage(`[1]`)})
	if err == nil || !strings.Contains(err.Error(), "payment") {
		t.Fatalf("unexpected decode error: %v", err)
	}
	if order().Endpoint != "first" {
		t.Fatal("failed reload published part of the extensions")
	}
}

func decodeExtend(t *testing.T, blob string) {
	t.Helper()
	var settings struct{ Extend map[string]json.RawMessage }
	if err := json.Unmarshal([]byte(blob), &settings); err != nil {
		t.Fatal(err)
	}
	values, err := prepareExtensions(settings.Extend)
	if err != nil {
		t.Fatal(err)
	}
	current.Store(&publishedSettings{settings: &Settings{}, extensions: values})
}

const extendSettingsTemplate = `settings:
  application:
    name: extend-test
  logger:
    level: error
  extend:
    order:
      Endpoint: %s
    payment:
      Endpoint: %s
`

func TestRegisterExtendSurvivesConfigReload(t *testing.T) {
	resetExtend(t)
	order := RegisterExtend[orderExtend]("order")
	payment := RegisterExtend[paymentExtend]("payment")
	path := filepath.Join(t.TempDir(), "settings.yml")
	writeExtendSettings(t, path, "v1-order", "v1-payment")
	watcher, err := Setup(t.Context(), file.NewSource(file.WithPath(path)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = watcher.Close() })
	stop := make(chan struct{})
	var readers sync.WaitGroup
	readers.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
				_ = order().Endpoint
				_ = payment().Endpoint
			}
		}
	})
	t.Cleanup(func() { close(stop); readers.Wait() })
	first := order()
	writeExtendSettings(t, path, "v2-order", "v2-payment")
	waitFor(t, 5*time.Second, func() bool { return order().Endpoint == "v2-order" && payment().Endpoint == "v2-payment" }, "extensions did not reload")
	if first.Endpoint != "v1-order" {
		t.Fatal("reload mutated a previously returned extension")
	}
}

func writeExtendSettings(t *testing.T, path, order, payment string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(fmt.Sprintf(extendSettingsTemplate, order, payment)), 0o600); err != nil {
		t.Fatal(err)
	}
}
