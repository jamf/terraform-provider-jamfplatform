// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package listhydration is the estate-free guard for the list resource's
// config-generation path. That path hands a flattener an empty model with
// includeUnmanaged set and expects every block and collection the wire carries
// to come back populated; a flattener that only fills a container the model
// already holds returns it null instead, and the generated configuration drops
// the value without a diagnostic (#520). Fill builds a wire value with every
// field populated and NilBlocks names what a flatten left null.
package listhydration

import (
	"encoding/xml"
	"reflect"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var xmlNameType = reflect.TypeFor[xml.Name]()

// Fill populates every field of the struct target points at: scalars take a
// fixed non-zero value, pointers are allocated, and each slice carries one
// element. xml.Name is left alone. Fill panics unless target is a non-nil
// pointer to a struct.
func Fill(target any) {
	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		panic("listhydration.Fill: target must be a non-nil pointer to a struct")
	}
	fill(v.Elem(), 0)
}

func fill(v reflect.Value, depth int) {
	if depth > 12 {
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		fill(v.Elem(), depth+1)
	case reflect.Struct:
		if v.Type() == xmlNameType {
			return
		}
		for _, f := range v.Fields() {
			if f.CanSet() {
				fill(f, depth+1)
			}
		}
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 1, 1)
		fill(s.Index(0), depth+1)
		v.Set(s)
	case reflect.String:
		v.SetString("1")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(1)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1)
	}
}

// NilBlocks returns the dotted Go field path of every nested block or
// collection left null in the struct model points at: nil struct pointers, nil
// slices and null framework collections. Scalars are not reported, since a
// derived scalar is legitimately null. Fields named in allowed are skipped.
func NilBlocks(model any, allowed ...string) []string {
	skip := map[string]bool{}
	for _, a := range allowed {
		skip[a] = true
	}
	var out []string
	walk(reflect.ValueOf(model), "", skip, &out)
	sort.Strings(out)
	return out
}

func walk(v reflect.Value, path string, skip map[string]bool, out *[]string) {
	for v.Kind() == reflect.Pointer && !v.IsNil() {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		name := path + v.Type().Field(i).Name
		if skip[name] {
			continue
		}
		switch f.Kind() {
		case reflect.Pointer:
			if f.Type().Elem().Kind() != reflect.Struct {
				continue
			}
			if f.IsNil() {
				*out = append(*out, name)
				continue
			}
			walk(f, name+".", skip, out)
		case reflect.Slice:
			if f.IsNil() {
				*out = append(*out, name)
			}
		case reflect.Struct, reflect.Interface:
			val, ok := f.Interface().(attr.Value)
			if !ok {
				if f.Kind() == reflect.Struct {
					walk(f, name+".", skip, out)
				}
				continue
			}
			if isCollection(val) && val.IsNull() {
				*out = append(*out, name)
			}
		}
	}
}

func isCollection(v attr.Value) bool {
	switch v.(type) {
	case basetypes.SetValue, basetypes.ListValue, basetypes.MapValue, basetypes.ObjectValue:
		return true
	}
	return false
}
