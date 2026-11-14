package runtime

import (
	"github.com/gin-gonic/gin"
	"testing"
)

func TestRegistrySnapshotsOwnTheirContainers(t *testing.T) {
	app := New()
	app.SetApp(DefaultTenant, "original")
	apps := app.Apps()
	apps[app.DefaultTenant()] = "changed"
	if app.App(app.DefaultTenant()) != "original" {
		t.Error("Apps exposed internal map")
	}
	app.SetCasbinExclude(DefaultTenant, "original")
	exclusions := app.CasbinExclusions()
	exclusions[app.DefaultTenant()] = "changed"
	if app.CasbinExclude(app.DefaultTenant()) != "original" {
		t.Error("CasbinExclusions exposed internal map")
	}
	original := map[string]any{"name": "original"}
	app.SetConfig(app.DefaultTenant(), original)
	original["name"] = "changed"
	if app.ConfigValue(DefaultTenant, "name") != "original" {
		t.Error("SetConfig retained caller map")
	}
	app.Config(DefaultTenant)["name"] = "changed again"
	if app.ConfigValue(DefaultTenant, "name") != "original" {
		t.Error("GetConfig exposed internal map")
	}
	app.Config(app.DefaultTenant())["name"] = "changed again"
	if app.ConfigValue(DefaultTenant, "name") != "original" {
		t.Error("Config exposed internal map")
	}
	calls := 0
	app.AddHandler(DefaultTenant, func(*gin.RouterGroup, ...*gin.HandlerFunc) { calls++ })
	app.Handlers(DefaultTenant)[0] = nil
	handlers := app.Handlers(app.DefaultTenant())
	if handlers[0] == nil {
		t.Fatal("GetHandler exposed internal slice")
	}
	handlers[0] = nil
	app.Handlers(DefaultTenant)[0](nil)
	if calls != 1 {
		t.Fatal("Handlers exposed internal slice")
	}
}
