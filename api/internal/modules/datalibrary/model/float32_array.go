package model

import (
	"database/sql/driver"

	"github.com/lib/pq"
)

// Float32Array stores PostgreSQL REAL[] values while exposing Go []float32 semantics.
type Float32Array []float32

func (a *Float32Array) Scan(src any) error {
	var values pq.Float32Array
	if err := values.Scan(src); err != nil {
		return err
	}
	*a = Float32Array(values)
	return nil
}

func (a Float32Array) Value() (driver.Value, error) {
	return pq.Float32Array(a).Value()
}
