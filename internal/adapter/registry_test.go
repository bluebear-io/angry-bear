// registry_test.go contains tests for the adapter registry and auto-detection logic.
package adapter

import (
	"testing"
)

func TestRegistryGet_Claude(t *testing.T) {
	reg := NewRegistry()

	adapter, err := reg.Get("claude")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter.Name() != "claude" {
		t.Errorf("Name() = %q, want %q", adapter.Name(), "claude")
	}
}

func TestRegistryGet_Unknown(t *testing.T) {
	reg := NewRegistry()

	_, err := reg.Get("unknown")
	if err == nil {
		t.Fatal("expected error for unknown adapter, got nil")
	}
}

func TestAutoDetect_Claude(t *testing.T) {
	reg := NewRegistry()
	input := []byte(`{"hook_event_name":"PreToolUse","session_id":"x","tool_name":"Edit"}`)

	adapter, err := reg.AutoDetect(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter.Name() != "claude" {
		t.Errorf("Name() = %q, want %q", adapter.Name(), "claude")
	}
}

func TestAutoDetect_CursorVersionDetected(t *testing.T) {
	reg := NewRegistry()
	// JSON with cursor_version should detect as cursor (even with hook_event_name)
	input := []byte(`{"hook_event_name":"PreToolUse","cursor_version":"0.50","tool_name":"Edit"}`)

	adapter, err := reg.AutoDetect(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter.Name() != "cursor" {
		t.Errorf("Name() = %q, want %q", adapter.Name(), "cursor")
	}
}

func TestAutoDetect_UnrecognizableInput(t *testing.T) {
	reg := NewRegistry()
	input := []byte(`{"some":"random","fields":"here"}`)

	_, err := reg.AutoDetect(input)
	if err == nil {
		t.Fatal("expected error for unrecognizable input, got nil")
	}
}

func TestAutoDetect_InvalidJSON(t *testing.T) {
	reg := NewRegistry()

	_, err := reg.AutoDetect([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestRegistryGet_Cursor(t *testing.T) {
	reg := NewRegistry()

	adapter, err := reg.Get("cursor")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter.Name() != "cursor" {
		t.Errorf("Name() = %q, want %q", adapter.Name(), "cursor")
	}
}

func TestAutoDetect_Cursor_BeforeFileEdit(t *testing.T) {
	reg := NewRegistry()
	input := []byte(`{"hook_event_name":"beforeFileEdit","conversation_id":"x","cursor_version":"0.48.1"}`)

	adapter, err := reg.AutoDetect(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter.Name() != "cursor" {
		t.Errorf("Name() = %q, want %q", adapter.Name(), "cursor")
	}
}

func TestAutoDetect_CursorPreferredOverClaude(t *testing.T) {
	reg := NewRegistry()
	// Both hook_event_name and cursor_version present -- cursor_version takes priority
	input := []byte(`{"hook_event_name":"preToolUse","conversation_id":"x","cursor_version":"0.48.1"}`)

	adapter, err := reg.AutoDetect(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter.Name() != "cursor" {
		t.Errorf("Name() = %q, want %q (cursor_version should take priority over hook_event_name)", adapter.Name(), "cursor")
	}
}

func TestRegistryNames(t *testing.T) {
	reg := NewRegistry()
	names := reg.Names()

	if len(names) < 2 {
		t.Fatalf("expected at least 2 adapters, got %d", len(names))
	}

	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	if !nameSet["claude"] {
		t.Error("missing 'claude' in Names()")
	}
	if !nameSet["cursor"] {
		t.Error("missing 'cursor' in Names()")
	}
}
