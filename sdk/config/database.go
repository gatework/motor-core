package config

// Database configures a database connection pool and optional resolvers.
type Database struct {
	Driver          string
	Source          string
	ConnMaxIdleTime int
	ConnMaxLifeTime int
	MaxIdleConns    int
	MaxOpenConns    int
	Registers       []DBResolverConfig
}

// DBResolverConfig selects source and replica connections for tables.
type DBResolverConfig struct {
	Sources  []string
	Replicas []string
	Policy   string
	Tables   []string
}
