package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ErrUnknownVar is returned when a template references an undefined variable.
type ErrUnknownVar struct {
	Var string
}

func (e ErrUnknownVar) Error() string {
	return fmt.Sprintf("template variable ${%s} is not defined", e.Var)
}

var varPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// Render substitutes ${VAR} tokens in tmpl with values from vars.
// Returns ErrUnknownVar if any token is not found in vars.
func Render(tmpl string, vars map[string]string) (string, error) {
	var renderErr error
	result := varPattern.ReplaceAllStringFunc(tmpl, func(match string) string {
		if renderErr != nil {
			return match
		}
		key := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
		val, ok := vars[key]
		if !ok {
			renderErr = ErrUnknownVar{Var: key}
			return match
		}
		return val
	})
	if renderErr != nil {
		return "", renderErr
	}
	return result, nil
}

// RenderFile reads the file at tmplPath and calls Render.
func RenderFile(tmplPath string, vars map[string]string) (string, error) {
	data, err := os.ReadFile(tmplPath)
	if err != nil {
		return "", fmt.Errorf("read template %q: %w", tmplPath, err)
	}
	return Render(string(data), vars)
}

// WriteFile renders tmplPath and writes the result to outPath atomically.
func WriteFile(tmplPath, outPath string, vars map[string]string, mode os.FileMode) error {
	content, err := RenderFile(tmplPath, vars)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("mkdir %q: %w", filepath.Dir(outPath), err)
	}
	tmp := outPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), mode); err != nil {
		return fmt.Errorf("write %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, outPath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %q → %q: %w", tmp, outPath, err)
	}
	return nil
}
