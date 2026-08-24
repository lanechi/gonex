package logging

import "time"

// Field is a structured logging field. The concrete value is converted to a
// typed Zap field by the built-in implementation.
type Field struct {
	Key   string
	Value any
}

func String(key, value string) Field      { return Field{Key: key, Value: value} }
func Int(key string, value int) Field     { return Field{Key: key, Value: value} }
func Int64(key string, value int64) Field { return Field{Key: key, Value: value} }
func Float64(key string, value float64) Field {
	return Field{Key: key, Value: value}
}
func Bool(key string, value bool) Field { return Field{Key: key, Value: value} }
func Duration(key string, value time.Duration) Field {
	return Field{Key: key, Value: value}
}
func Time(key string, value time.Time) Field { return Field{Key: key, Value: value} }
func Any(key string, value any) Field        { return Field{Key: key, Value: value} }
func Error(err error) Field                  { return Field{Key: "error", Value: err} }
