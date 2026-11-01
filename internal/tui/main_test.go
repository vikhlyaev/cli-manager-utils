package tui

import (
	"os"
	"testing"
)

// TestMain убирает $ZDOTDIR: тесты подменяют HOME и ждут .zshrc именно там.
func TestMain(m *testing.M) {
	os.Unsetenv("ZDOTDIR")
	os.Exit(m.Run())
}
