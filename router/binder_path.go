package router

import (
	"reflect"

	"github.com/gin-gonic/gin"
)

func bindPath(target reflect.Value, context *gin.Context, field FieldBinding) (bool, error) {
	if field.Path == "" {
		return false, nil
	}
	value := context.Param(field.Path)
	if value == "" {
		return false, nil
	}
	return bindValues(target, field, field.Path, []string{value})
}
