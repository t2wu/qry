package mdl

import (
	"fmt"
	"reflect"
	"runtime/debug"
	"strings"
)

func IsFieldInModel(modelObj IModel, field string) bool {
	toks := strings.SplitN(field, ".", 2)
	first := toks[0]

	structField, ok := reflect.TypeOf(modelObj).Elem().FieldByName(first)

	getInnerType := func(structField reflect.StructField) reflect.Type {
		if structField.Type.Kind() == reflect.Ptr || structField.Type.Kind() == reflect.Slice {
			return structField.Type.Elem() // pointer or slice both use Elem(0)
		} else {
			return structField.Type
		}
	}

	ok2 := true
	if ok && len(toks) > 1 && toks[1] != "" { // has nested field
		innerType := getInnerType(structField)
		innerModel := reflect.New(innerType).Interface().(IModel)

		ok2 = IsFieldInModel(innerModel, toks[1])
	}

	return ok && ok2
}

func GetInnerModelIfValid(modelObj IModel, field string) (IModel, error) {
	typ, err := GetModelFieldTypeInModelIfValid(modelObj, field)
	if err != nil {
		return nil, err
	}

	m, ok := reflect.New(typ).Interface().(IModel)
	if !ok {
		return nil, fmt.Errorf("not an IModel")
	}
	return m, nil
}

// Never returns the pointer value
// Since what we want is reflec.New() and it would be a pointer
func GetModelFieldTypeInModelIfValid(modelObj IModel, field string) (reflect.Type, error) {
	toks := strings.SplitN(field, ".", 2)
	first := toks[0]

	structField, ok := reflect.TypeOf(modelObj).Elem().FieldByName(first)
	if !ok {
		debug.PrintStack()
		return nil, fmt.Errorf("invalid field")
	}

	typ := structField.Type

	if structField.Type.Kind() == reflect.Ptr || structField.Type.Kind() == reflect.Slice {
		typ = structField.Type.Elem()
	}

	// If it is still pointer follows it
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	var err error
	if ok && len(toks) > 1 && toks[1] != "" { // has nested field
		innerModel := reflect.New(typ).Interface().(IModel)

		typ, err = GetModelFieldTypeInModelIfValid(innerModel, toks[1])
	}

	return typ, err
}

// FieldNotInModelError is for GetModelFieldTypeIfValid.
// if field doesn't exist in the mdl, return this error
// We want to go ahead and skip it since this field may be other
// options that user can read in hookpoints
type FieldNotInModelError struct {
	Msg string
}

func (r *FieldNotInModelError) Error() string {
	return r.Msg
}
