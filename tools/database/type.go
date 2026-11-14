package database

import (
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

var policies = map[string]dbresolver.Policy{
	"random": dbresolver.RandomPolicy{},
}

// ResolverConfigure adds a source/replica resolver configuration.
type ResolverConfigure interface {
	Init(register *dbresolver.DBResolver, open func(string) gorm.Dialector) *dbresolver.DBResolver
}
