package runtime

import (
	"maps"
	"net/http"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/casbin/casbin/v3"
	"github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/storage"
	"github.com/gatework/motor-core/storage/queue"
	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

const DefaultTenant = "default"

type Config map[string]any
type Handler func(*gin.RouterGroup, ...*gin.HandlerFunc)
type Router struct{ HTTPMethod, RelativePath, Handler string }

// Application stores host resources. Returned containers are snapshots; the
// database, scheduler and other resource values retain their own identity.
type Application struct {
	mux           sync.RWMutex
	dbs           map[string]*gorm.DB
	casbins       map[string]*casbin.SyncedEnforcer
	crontab       map[string]*cron.Cron
	app           map[string]any
	casbinExclude map[string]any
	middlewares   map[string]any
	handler       map[string][]Handler
	configs       map[string]Config
	engine        http.Handler
	routers       []Router
	logger        logger.Logger
	cache         storage.Cache
	queue         storage.Queue
	memoryQueue   storage.Queue
	defaultTenant string
	appRouters    registry
	before        registry
	phases        [phaseCount]registry
	shuttingDown  atomic.Bool
}

func New() *Application {
	return &Application{
		dbs: make(map[string]*gorm.DB), casbins: make(map[string]*casbin.SyncedEnforcer),
		crontab: make(map[string]*cron.Cron), app: make(map[string]any),
		casbinExclude: make(map[string]any), middlewares: make(map[string]any),
		handler: make(map[string][]Handler), configs: make(map[string]Config),
		memoryQueue: queue.New(10000), defaultTenant: DefaultTenant,
	}
}

func put[K comparable, V any](m *map[K]V, k K, v V) {
	if *m == nil {
		*m = make(map[K]V)
	}
	(*m)[k] = v
}
func tenantValue[V any](m map[string]V, tenant string) V {
	if v, ok := m["*"]; ok {
		return v
	}
	return m[tenant]
}
func (a *Application) SetDB(tenant string, db *gorm.DB) {
	a.mux.Lock()
	defer a.mux.Unlock()
	put(&a.dbs, tenant, db)
}
func (a *Application) DB(tenant string) *gorm.DB {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return tenantValue(a.dbs, tenant)
}
func (a *Application) Databases() map[string]*gorm.DB {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return maps.Clone(a.dbs)
}
func (a *Application) SetApp(tenant string, v any) {
	a.mux.Lock()
	defer a.mux.Unlock()
	put(&a.app, tenant, v)
}
func (a *Application) App(tenant string) any {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return tenantValue(a.app, tenant)
}
func (a *Application) Apps() map[string]any {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return maps.Clone(a.app)
}
func (a *Application) SetCasbinExclude(tenant string, v any) {
	a.mux.Lock()
	defer a.mux.Unlock()
	put(&a.casbinExclude, tenant, v)
}
func (a *Application) CasbinExclude(tenant string) any {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return tenantValue(a.casbinExclude, tenant)
}
func (a *Application) CasbinExclusions() map[string]any {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return maps.Clone(a.casbinExclude)
}
func (a *Application) SetCasbin(tenant string, v *casbin.SyncedEnforcer) {
	a.mux.Lock()
	defer a.mux.Unlock()
	put(&a.casbins, tenant, v)
}
func (a *Application) Casbin(tenant string) *casbin.SyncedEnforcer {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return tenantValue(a.casbins, tenant)
}
func (a *Application) Enforcers() map[string]*casbin.SyncedEnforcer {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return maps.Clone(a.casbins)
}
func (a *Application) SetCrontab(tenant string, v *cron.Cron) {
	a.mux.Lock()
	defer a.mux.Unlock()
	put(&a.crontab, tenant, v)
}
func (a *Application) Crontab(tenant string) *cron.Cron {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return tenantValue(a.crontab, tenant)
}
func (a *Application) Crontabs() map[string]*cron.Cron {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return maps.Clone(a.crontab)
}
func (a *Application) SetEngine(v http.Handler) { a.mux.Lock(); defer a.mux.Unlock(); a.engine = v }
func (a *Application) Engine() http.Handler     { a.mux.RLock(); defer a.mux.RUnlock(); return a.engine }
func (a *Application) Routers() []Router {
	engine := a.Engine()
	ge, ok := engine.(*gin.Engine)
	if !ok {
		a.mux.RLock()
		defer a.mux.RUnlock()
		return slices.Clone(a.routers)
	}
	routes := ge.Routes()
	out := make([]Router, 0, len(routes))
	for _, r := range routes {
		out = append(out, Router{HTTPMethod: r.Method, RelativePath: r.Path, Handler: r.Handler})
	}
	a.mux.Lock()
	a.routers = slices.Clone(out)
	a.mux.Unlock()
	return out
}
func (a *Application) SetLogger(v logger.Logger) { a.mux.Lock(); defer a.mux.Unlock(); a.logger = v }
func (a *Application) Logger() logger.Logger {
	a.mux.RLock()
	v := a.logger
	a.mux.RUnlock()
	if v == nil {
		return logger.Default()
	}
	return v
}
func (a *Application) SetDefaultTenant(v string) {
	a.mux.Lock()
	defer a.mux.Unlock()
	a.defaultTenant = v
}
func (a *Application) DefaultTenant() string {
	a.mux.RLock()
	defer a.mux.RUnlock()
	if a.defaultTenant == "" {
		return DefaultTenant
	}
	return a.defaultTenant
}
func (a *Application) SetMiddleware(k string, v any) {
	a.mux.Lock()
	defer a.mux.Unlock()
	put(&a.middlewares, k, v)
}
func (a *Application) Middleware(k string) any {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return a.middlewares[k]
}
func (a *Application) Middlewares() map[string]any {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return maps.Clone(a.middlewares)
}
func (a *Application) SetCache(v storage.Cache) { a.mux.Lock(); defer a.mux.Unlock(); a.cache = v }
func (a *Application) Cache() storage.Cache     { a.mux.RLock(); defer a.mux.RUnlock(); return a.cache }
func (a *Application) SetQueue(v storage.Queue) { a.mux.Lock(); defer a.mux.Unlock(); a.queue = v }
func (a *Application) Queue() storage.Queue {
	a.mux.Lock()
	defer a.mux.Unlock()
	if a.queue != nil {
		return a.queue
	}
	if a.memoryQueue == nil {
		a.memoryQueue = queue.New(10000)
	}
	return a.memoryQueue
}
func (a *Application) QueueForTenant(tenant string) *Queue { return NewQueue(tenant, a.Queue()) }
func (a *Application) AddHandler(tenant string, h Handler) {
	a.mux.Lock()
	defer a.mux.Unlock()
	if a.handler == nil {
		a.handler = make(map[string][]Handler)
	}
	a.handler[tenant] = append(a.handler[tenant], h)
}
func (a *Application) Handlers(tenant string) []Handler {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return slices.Clone(a.handler[tenant])
}
func (a *Application) AllHandlers() map[string][]Handler {
	a.mux.RLock()
	defer a.mux.RUnlock()
	out := make(map[string][]Handler, len(a.handler))
	for k, v := range a.handler {
		out[k] = slices.Clone(v)
	}
	return out
}
func (a *Application) SetConfig(tenant string, v map[string]any) {
	a.mux.Lock()
	defer a.mux.Unlock()
	put(&a.configs, tenant, Config(maps.Clone(v)))
}
func (a *Application) Config(tenant string) map[string]any {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return maps.Clone(a.configs[tenant])
}
func (a *Application) SetConfigValue(tenant, key string, v any) {
	a.mux.Lock()
	defer a.mux.Unlock()
	if a.configs == nil {
		a.configs = make(map[string]Config)
	}
	if a.configs[tenant] == nil {
		a.configs[tenant] = make(Config)
	}
	a.configs[tenant][key] = v
}
func (a *Application) ConfigValue(tenant, key string) any {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return a.configs[tenant][key]
}
