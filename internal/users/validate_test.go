package users

import (
	"errors"
	"testing"
)

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"alice", false},
		{"alice_1", false},
		{"alice-1", false},
		{"", true},
		{"Alice", true},
		{"-alice", true},
		{"alice.example", true},
		{"root", true},
	}
	for _, tt := range tests {
		err := ValidateUsername(tt.name)
		if tt.wantErr && !errors.Is(err, ErrInvalidUsername) {
			t.Errorf("%q: expected ErrInvalidUsername, got %v", tt.name, err)
		}
		if !tt.wantErr && err != nil {
			t.Errorf("%q: expected nil, got %v", tt.name, err)
		}
	}
}
