// Package state provides SQLite-backed persistent storage for the overlay.
package state

import "time"

// Admin represents the configured overlay administrator.
type Admin struct {
	Username string
	UID      int
	SetAt    time.Time
	SetBy    string
}
