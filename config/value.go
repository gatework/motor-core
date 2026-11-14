package config

import (
	"maps"
	"slices"
	"time"

	"github.com/gatework/motor-core/config/reader"
)

type value struct{}

func newValue() reader.Value {
	return new(value)
}

func (v *value) Bool(def bool) bool {
	return def
}

func (v *value) Int(def int) int {
	return def
}

func (v *value) String(def string) string {
	return def
}

func (v *value) Float64(def float64) float64 {
	return def
}

func (v *value) Duration(def time.Duration) time.Duration {
	return def
}

func (v *value) StringSlice(def []string) []string {
	return slices.Clone(def)
}

func (v *value) StringMap(def map[string]string) map[string]string {
	return maps.Clone(def)
}

func (v *value) Scan(val any) error {
	return nil
}

func (v *value) Bytes() []byte {
	return nil
}
