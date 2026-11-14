package config

// Application configures the host HTTP server and application identity.
type Application struct {
	ReadTimeout   int
	WriterTimeout int
	Host          string
	Port          int64
	Name          string
	Mode          string
	DemoMsg       string
	EnableDP      bool
	// 租户模式：1：单租户 2：多租户
	TenantMode int `yaml:"tenantMode"`
}
