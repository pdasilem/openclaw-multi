package main

import "testing"

func TestRunHelp(t *testing.T) {
	if err := run([]string{"--help"}); err != nil {
		t.Fatalf("run --help: %v", err)
	}
}

func TestRunInvalidFlag(t *testing.T) {
	if err := run([]string{"--missing"}); err == nil {
		t.Fatal("expected invalid flag error")
	}
}
