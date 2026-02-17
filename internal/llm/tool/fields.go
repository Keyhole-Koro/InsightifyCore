package llmtool

import (
	"fmt"
	"reflect"
	"strings"
)

// FieldOptions controls how struct fields map to PromptField.
type FieldOptions struct {
	NameTag         string
	DescTag         string
	TypeTag         string
	PromptTag       string
	RequiredDefault bool
}

// DefaultFieldOptions returns the standard tag mapping.
func DefaultFieldOptions() FieldOptions {
	return FieldOptions{
		NameTag:         "json",
		DescTag:         "prompt_desc",
		TypeTag:         "prompt_type",
		PromptTag:       "prompt",
		RequiredDefault: true,
	}
}

// FieldsFromStruct builds prompt fields from a Go struct using tags.
func FieldsFromStruct(v any, opts ...FieldOptions) ([]PromptField, error) {
	if v == nil {
		return nil, fmt.Errorf("llmtool: struct is nil")
	}
	cfg := DefaultFieldOptions()
	if len(opts) > 0 {
		cfg = opts[0]
	}
	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("llmtool: expected struct, got %s", t.Kind())
	}
	fields := make([]PromptField, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		if skip := shouldSkipField(f, cfg.PromptTag); skip {
			continue
		}
		name := fieldName(f, cfg.NameTag)
		if name == "" {
			continue
		}
		required := cfg.RequiredDefault
		if r, ok := requiredOverride(f, cfg.PromptTag); ok {
			required = r
		}
		typ := fieldType(f, cfg.TypeTag)
		desc := strings.TrimSpace(f.Tag.Get(cfg.DescTag))
		fields = append(fields, PromptField{
			Name:        name,
			Type:        typ,
			Required:    required,
			Description: desc,
		})
	}
	return fields, nil
}

// MustFieldsFromStruct panics on error; useful for prompt spec literals.
func MustFieldsFromStruct(v any, opts ...FieldOptions) []PromptField {
	fields, err := FieldsFromStruct(v, opts...)
	if err != nil {
		panic(err)
	}
	return fields
}

func shouldSkipField(f reflect.StructField, promptTag string) bool {
	tag := strings.TrimSpace(f.Tag.Get(promptTag))
	if tag == "" {
		return false
	}
	for _, part := range strings.Split(tag, ",") {
		part = strings.TrimSpace(part)
		if part == "-" || part == "omit" {
			return true
		}
	}
	return false
}

func requiredOverride(f reflect.StructField, promptTag string) (bool, bool) {
	tag := strings.TrimSpace(f.Tag.Get(promptTag))
	if tag == "" {
		return false, false
	}
	for _, part := range strings.Split(tag, ",") {
		part = strings.TrimSpace(part)
		switch part {
		case "required":
			return true, true
		case "optional":
			return false, true
		}
	}
	return false, false
}

func fieldName(f reflect.StructField, nameTag string) string {
	tag := strings.TrimSpace(f.Tag.Get(nameTag))
	if tag != "" {
		name := strings.Split(tag, ",")[0]
		if name == "-" {
			return ""
		}
		if name != "" {
			return name
		}
	}
	return toSnake(f.Name)
}

func fieldType(f reflect.StructField, typeTag string) string {
	tag := strings.TrimSpace(f.Tag.Get(typeTag))
	if tag != "" {
		return tag
	}
	return typeString(f.Type)
}

func typeString(t reflect.Type) string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "bool"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "int"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "uint"
	case reflect.Float32, reflect.Float64:
		return "float64"
	case reflect.Slice:
		return "[]" + typeString(t.Elem())
	case reflect.Array:
		return fmt.Sprintf("[%d]%s", t.Len(), typeString(t.Elem()))
	case reflect.Map:
		return fmt.Sprintf("map[%s]%s", typeString(t.Key()), typeString(t.Elem()))
	case reflect.Struct:
		if t.Name() != "" {
			return t.Name()
		}
		return "object"
	case reflect.Interface:
		return "any"
	default:
		return t.Kind().String()
	}
}

func toSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			prev := rune(s[i-1])
			next := rune(0)
			if i+1 < len(s) {
				next = rune(s[i+1])
			}
			if prev >= 'a' && prev <= 'z' || (next >= 'a' && next <= 'z') {
				b.WriteByte('_')
			}
		}
		b.WriteRune(toLower(r))
	}
	return b.String()
}

func toLower(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}
