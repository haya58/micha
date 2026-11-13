package persona

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDefault(t *testing.T) {
	p, err := Load("")
	if err != nil {
		t.Fatalf("Load default: %v", err)
	}
	if !strings.Contains(p, "persona") && !strings.Contains(p, "Persona") && len(p) < 100 {
		t.Fatalf("default persona looks wrong: %.100q", p)
	}
}

func TestLoadCustom(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persona.md")
	if err := os.WriteFile(path, []byte("custom persona text"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Load(path)
	if err != nil {
		t.Fatalf("Load custom: %v", err)
	}
	if p != "custom persona text" {
		t.Fatalf("got %q", p)
	}
}

func TestLoadMissing(t *testing.T) {
	if _, err := Load("/nonexistent/persona.md"); err == nil {
		t.Fatal("expected error for missing file")
	}
}
