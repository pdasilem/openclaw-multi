package systemprep

import (
	"bufio"
	"strings"
	"testing"
)

func TestAskExistingConfigAction(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want existingConfigAction
	}{
		{name: "default keep edit", in: "\n", want: existingConfigKeep},
		{name: "keep edit", in: "k\n", want: existingConfigKeep},
		{name: "replace", in: "r\n", want: existingConfigReplace},
		{name: "retry invalid answer", in: "x\nreplace\n", want: existingConfigReplace},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := askExistingConfigAction(bufio.NewReader(strings.NewReader(tt.in)), "/tmp/config.yml")
			if err != nil {
				t.Fatalf("askExistingConfigAction returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("askExistingConfigAction = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAskExistingConfigActionAbort(t *testing.T) {
	_, err := askExistingConfigAction(bufio.NewReader(strings.NewReader("a\n")), "/tmp/config.yml")
	if err == nil {
		t.Fatal("expected abort error")
	}
	if !strings.Contains(err.Error(), "aborted while reconciling /tmp/config.yml") {
		t.Fatalf("unexpected abort error: %v", err)
	}
}
