package utils

import (
	"database/sql/driver"
	"fmt"
	"time"
)

// JSONTime preserves the SQL timestamp JSON format.
//
//nolint:recvcheck // Value hooks preserve JSON/SQL encoding of values; Scan must mutate the destination.
type JSONTime struct {
	time.Time
}

// MarshalJSON on JSONTime format Time field with %Y-%m-%d %H:%M:%S
func (t JSONTime) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte(`""`), nil
	}
	return []byte(t.Format(`"2006-01-02 15:04:05"`)), nil
}

// Value insert timestamp into mysql need this function.
func (t JSONTime) Value() (driver.Value, error) {
	if t.IsZero() {
		return nil, nil //nolint:nilnil // database/sql/driver.Value requires (nil, nil) to represent SQL NULL.
	}
	return t.Time, nil
}

// Scan valueof time.Time
func (t *JSONTime) Scan(v any) error {
	value, ok := v.(time.Time)
	if ok {
		*t = JSONTime{Time: value}
		return nil
	}
	return fmt.Errorf("utils: cannot convert %T to timestamp", v)
}
