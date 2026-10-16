package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// EnvPrefix is prepended to override keys. Nesting uses "__" between YAML keys.
// Example: DBT_ESYNC_SINK__GIT__TOKEN → sink.git.token
const EnvPrefix = "DBT_ESYNC_"

// ApplyEnvOverrides overlays DBT_ESYNC_* environment variables onto cfg.
// Env wins over YAML. Unknown paths and type mismatches return an error.
func ApplyEnvOverrides(cfg *Config) error {
	return applyEnvOverrides(cfg, os.Environ())
}

func applyEnvOverrides(cfg *Config, environ []string) error {
	for _, e := range environ {
		key, val, ok := strings.Cut(e, "=")
		if !ok || !strings.HasPrefix(key, EnvPrefix) {
			continue
		}
		path := strings.TrimPrefix(key, EnvPrefix)
		if path == "" {
			return fmt.Errorf("env %s: empty path", key)
		}
		segments := strings.Split(path, "__")
		for i, s := range segments {
			segments[i] = strings.ToLower(s)
		}
		if err := setByYAMLPath(reflect.ValueOf(cfg), segments, val); err != nil {
			return fmt.Errorf("env %s: %w", key, err)
		}
	}
	return nil
}

func setByYAMLPath(v reflect.Value, segments []string, raw string) error {
	if len(segments) == 0 {
		return fmt.Errorf("empty path")
	}
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("cannot descend into %s", v.Kind())
	}

	field, ok := fieldByYAMLTag(v, segments[0])
	if !ok {
		return fmt.Errorf("unknown field %q", segments[0])
	}

	if len(segments) == 1 {
		return setLeaf(field, raw)
	}

	for field.Kind() == reflect.Pointer {
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		field = field.Elem()
	}
	return setByYAMLPath(field.Addr(), segments[1:], raw)
}

func fieldByYAMLTag(structVal reflect.Value, key string) (reflect.Value, bool) {
	t := structVal.Type()
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		tag := sf.Tag.Get("yaml")
		if tag == "" || tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name == key {
			return structVal.Field(i), true
		}
	}
	return reflect.Value{}, false
}

func setLeaf(field reflect.Value, raw string) error {
	for field.Kind() == reflect.Pointer {
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		field = field.Elem()
	}
	if !field.CanSet() {
		return fmt.Errorf("unsettable field")
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(raw)
	case reflect.Bool:
		b, err := parseEnvBool(raw)
		if err != nil {
			return err
		}
		field.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("int: %w", err)
		}
		field.SetInt(n)
	case reflect.Slice:
		if field.Type().Elem().Kind() != reflect.String {
			return fmt.Errorf("unsupported slice element %s", field.Type().Elem().Kind())
		}
		if raw == "" {
			field.Set(reflect.ValueOf([]string{}))
			return nil
		}
		parts := strings.Split(raw, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			out = append(out, strings.TrimSpace(p))
		}
		field.Set(reflect.ValueOf(out))
	default:
		return fmt.Errorf("unsupported kind %s", field.Kind())
	}
	return nil
}

// parseEnvBool accepts only true/false/1/0 (documented surface; not full strconv.ParseBool).
func parseEnvBool(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "1":
		return true, nil
	case "false", "0":
		return false, nil
	default:
		return false, fmt.Errorf("bool: want true/false/1/0, got %q", raw)
	}
}
