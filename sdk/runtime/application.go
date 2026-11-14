package runtime

import (
	"maps"
	"net/http"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/casbin/casbin/v3"
	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/storage"
	"github.com/gatework/motor-core/storage/queue"
)

// DefaultTenant names the default resource namespace.
const DefaultTenant = "default"

// Config stores tenant configuration values.
type Config map[string]any

// Handler registers a tenant route group.
type Handler func(*gin.RouterGroup, ...*gin.HandlerFunc)

// Router describes one registered HTTP route.
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

// New constructs an application with empty resource registries.
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

// SetDB publishes the database for a tenant; ownership remains with the caller.
func (a *Application) SetDB(tenant string, db *gorm.DB) {
	a.mux.Lock()
	defer a.mux.Unlock()
	put(&a.dbs, tenant, db)
}

// DB returns the wildcard or tenant database.
func (a *Application) DB(tenant string) *gorm.DB {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return tenantValue(a.dbs, tenant)
}

// Databases returns a snapshot of the database registry.
func (a *Application) Databases() map[string]*gorm.DB {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return maps.Clone(a.dbs)
}

// SetApp publishes an application value for a tenant.
func (a *Application) SetApp(tenant string, v any) {
	a.mux.Lock()
	defer a.mux.Unlock()
	put(&a.app, tenant, v)
}

// App returns the wildcard or tenant application value.
func (a *Application) App(tenant string) any {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return tenantValue(a.app, tenant)
}

// Apps returns a snapshot of registered application values.
func (a *Application) Apps() map[string]any {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return maps.Clone(a.app)
}

// SetCasbinExclude publishes excluded authorization routes for a tenant.
func (a *Application) SetCasbinExclude(tenant string, v any) {
	a.mux.Lock()
	defer a.mux.Unlock()
	put(&a.casbinExclude, tenant, v)
}

// CasbinExclude returns the wildcard or tenant authorization exclusions.
func (a *Application) CasbinExclude(tenant string) any {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return tenantValue(a.casbinExclude, tenant)
}

// CasbinExclusions returns a snapshot of authorization exclusions.
func (a *Application) CasbinExclusions() map[string]any {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return maps.Clone(a.casbinExclude)
}

// SetCasbin publishes an authorization enforcer for a tenant.
func (a *Application) SetCasbin(tenant string, v *casbin.SyncedEnforcer) {
	a.mux.Lock()
	defer a.mux.Unlock()
	put(&a.casbins, tenant, v)
}

// Casbin returns the wildcard or tenant authorization enforcer.
func (a *Application) Casbin(tenant string) *casbin.SyncedEnforcer {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return tenantValue(a.casbins, tenant)
}

// Enforcers returns a snapshot of authorization enforcers.
func (a *Application) Enforcers() map[string]*casbin.SyncedEnforcer {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return maps.Clone(a.casbins)
}

// SetCrontab publishes a scheduler for a tenant without transferring ownership.
func (a *Application) SetCrontab(tenant string, v *cron.Cron) {
	a.mux.Lock()
	defer a.mux.Unlock()
	put(&a.crontab, tenant, v)
}

// Crontab returns the wildcard or tenant scheduler.
func (a *Application) Crontab(tenant string) *cron.Cron {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return tenantValue(a.crontab, tenant)
}

// Crontabs returns a snapshot of schedulers.
func (a *Application) Crontabs() map[string]*cron.Cron {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return maps.Clone(a.crontab)
}

// SetEngine publishes the host HTTP handler.
func (a *Application) SetEngine(v http.Handler) { a.mux.Lock(); defer a.mux.Unlock(); a.engine = v }

// Engine returns the host HTTP handler.
func (a *Application) Engine() http.Handler { a.mux.RLock(); defer a.mux.RUnlock(); return a.engine }

// Routers returns a snapshot of registered routes.
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

// SetLogger publishes a logger without transferring ownership.
func (a *Application) SetLogger(v logger.Logger) { a.mux.Lock(); defer a.mux.Unlock(); a.logger = v }

// Logger returns the configured application logger.
func (a *Application) Logger() logger.Logger {
	a.mux.RLock()
	v := a.logger
	a.mux.RUnlock()
	if v == nil {
		return logger.Default()
	}
	return v
}

// SetDefaultTenant sets the tenant used for default resource lookups.
func (a *Application) SetDefaultTenant(v string) {
	a.mux.Lock()
	defer a.mux.Unlock()
	a.defaultTenant = v
}

// DefaultTenant returns the tenant used for default resource lookups.
func (a *Application) DefaultTenant() string {
	a.mux.RLock()
	defer a.mux.RUnlock()
	if a.defaultTenant == "" {
		return DefaultTenant
	}
	return a.defaultTenant
}

// SetMiddleware publishes middleware under its registration key.
func (a *Application) SetMiddleware(k string, v any) {
	a.mux.Lock()
	defer a.mux.Unlock()
	put(&a.middlewares, k, v)
}

// Middleware returns the middleware stored under a key.
func (a *Application) Middleware(k string) any {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return a.middlewares[k]
}

// Middlewares returns a snapshot of middleware registrations.
func (a *Application) Middlewares() map[string]any {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return maps.Clone(a.middlewares)
}

// SetCache publishes a cache without transferring ownership.
func (a *Application) SetCache(v storage.Cache) { a.mux.Lock(); defer a.mux.Unlock(); a.cache = v }

// Cache returns the configured cache.
func (a *Application) Cache() storage.Cache { a.mux.RLock(); defer a.mux.RUnlock(); return a.cache }

// SetQueue publishes a queue without transferring ownership.
func (a *Application) SetQueue(v storage.Queue) { a.mux.Lock(); defer a.mux.Unlock(); a.queue = v }

// Queue returns the configured queue.
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

// QueueForTenant returns a tenant-tagging view borrowing the backing queue.
func (a *Application) QueueForTenant(tenant string) *Queue { return NewQueue(tenant, a.Queue()) }

// AddHandler registers a route-group handler for a tenant.
func (a *Application) AddHandler(tenant string, h Handler) {
	a.mux.Lock()
	defer a.mux.Unlock()
	if a.handler == nil {
		a.handler = make(map[string][]Handler)
	}
	a.handler[tenant] = append(a.handler[tenant], h)
}

// Handlers returns a snapshot of handlers for a tenant.
func (a *Application) Handlers(tenant string) []Handler {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return slices.Clone(a.handler[tenant])
}

// AllHandlers returns a snapshot of all handler registrations.
func (a *Application) AllHandlers() map[string][]Handler {
	a.mux.RLock()
	defer a.mux.RUnlock()
	out := make(map[string][]Handler, len(a.handler))
	for k, v := range a.handler {
		out[k] = slices.Clone(v)
	}
	return out
}

// SetConfig copies and publishes configuration values for a tenant.
func (a *Application) SetConfig(tenant string, v map[string]any) {
	a.mux.Lock()
	defer a.mux.Unlock()
	put(&a.configs, tenant, Config(maps.Clone(v)))
}

// Config returns a snapshot of tenant configuration values.
func (a *Application) Config(tenant string) map[string]any {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return maps.Clone(a.configs[tenant])
}

// SetConfigValue updates one tenant configuration value.
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

// ConfigValue returns one tenant configuration value.
func (a *Application) ConfigValue(tenant, key string) any {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return a.configs[tenant][key]
}
