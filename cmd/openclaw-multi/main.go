package main

import (
	"fmt"
	"os"

	"github.com/pdasilem/openclaw-multi/internal/tui"
)

func main() {
	if err := tui.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}
