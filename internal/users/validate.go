// Package users orchestrates managed OpenClaw user lifecycle operations.
package users

import (
	"errors"
	"fmt"
	"regexp"
)

var (
	usernamePattern = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)
	reservedUsers   = map[string]struct{}{
		"root":           {},
		"openclaw":       {},
		"openclaw-multi": {},
	}
)

// ErrInvalidUsername is returned when username cannot be safely passed to useradd.
var ErrInvalidUsername = errors.New("invalid username")

// ValidateUsername checks the conservative Linux username subset supported by the overlay.
func ValidateUsername(username string) error {
	switch {
	case username == "":
		return fmt.Errorf("%w: empty value", ErrInvalidUsername)
	case username[0] == '-':
		return fmt.Errorf("%w: must not start with '-'", ErrInvalidUsername)
	case !usernamePattern.MatchString(username):
		return fmt.Errorf("%w: use 1-32 lowercase letters, digits, '_' or '-'", ErrInvalidUsername)
	}
	if _, ok := reservedUsers[username]; ok {
		return fmt.Errorf("%w: %q is reserved", ErrInvalidUsername, username)
	}
	return nil
}
