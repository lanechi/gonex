package logging

import (
	"time"

	"go.uber.org/zap"
)

func toZapFields(fields []Field) []zap.Field {
	if len(fields) == 0 {
		return nil
	}
	converted := make([]zap.Field, 0, len(fields))
	for _, field := range fields {
		if field.Key == "" {
			continue
		}
		switch value := field.Value.(type) {
		case string:
			converted = append(converted, zap.String(field.Key, value))
		case int:
			converted = append(converted, zap.Int(field.Key, value))
		case int8:
			converted = append(converted, zap.Int8(field.Key, value))
		case int16:
			converted = append(converted, zap.Int16(field.Key, value))
		case int32:
			converted = append(converted, zap.Int32(field.Key, value))
		case int64:
			converted = append(converted, zap.Int64(field.Key, value))
		case uint:
			converted = append(converted, zap.Uint(field.Key, value))
		case uint8:
			converted = append(converted, zap.Uint8(field.Key, value))
		case uint16:
			converted = append(converted, zap.Uint16(field.Key, value))
		case uint32:
			converted = append(converted, zap.Uint32(field.Key, value))
		case uint64:
			converted = append(converted, zap.Uint64(field.Key, value))
		case float32:
			converted = append(converted, zap.Float32(field.Key, value))
		case float64:
			converted = append(converted, zap.Float64(field.Key, value))
		case bool:
			converted = append(converted, zap.Bool(field.Key, value))
		case time.Duration:
			converted = append(converted, zap.Duration(field.Key, value))
		case time.Time:
			converted = append(converted, zap.Time(field.Key, value))
		case error:
			converted = append(converted, zap.NamedError(field.Key, value))
		default:
			converted = append(converted, zap.Any(field.Key, value))
		}
	}
	return converted
}
