package controller

import (
	"strings"
	"testing"
	"time"

	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/configdiff"
	"github.com/go-logr/logr"
)

// --- buildSetConfigCommand tests ---

func TestBuildSetConfigCommand_ServiceContext(t *testing.T) {
	change := configdiff.Change{
		Path:     "service.proto-fd-max",
		Context:  "service",
		Key:      "proto-fd-max",
		NewValue: 20000,
	}

	cmd, err := buildSetConfigCommand(change)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "set-config:context=service;proto-fd-max=20000"
	if cmd != expected {
		t.Errorf("buildSetConfigCommand = %q, want %q", cmd, expected)
	}
}

func TestBuildSetConfigCommand_NamespaceContext(t *testing.T) {
	change := configdiff.Change{
		Path:      "namespace.default-ttl",
		Context:   "namespace",
		Key:       "default-ttl",
		NewValue:  3600,
		Namespace: "myns",
	}

	cmd, err := buildSetConfigCommand(change)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "set-config:context=namespace;id=myns;default-ttl=3600"
	if cmd != expected {
		t.Errorf("buildSetConfigCommand = %q, want %q", cmd, expected)
	}
}

func TestBuildSetConfigCommand_NetworkContext(t *testing.T) {
	change := configdiff.Change{
		Path:     "network.heartbeat.interval",
		Context:  "network",
		Key:      "heartbeat.interval",
		NewValue: 250,
	}

	cmd, err := buildSetConfigCommand(change)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "set-config:context=network;heartbeat.interval=250"
	if cmd != expected {
		t.Errorf("buildSetConfigCommand = %q, want %q", cmd, expected)
	}
}

// TestDiffToSetConfigCommand_HeartbeatDynamic is the end-to-end regression
// test for the dynamic heartbeat/fabric Key bug. It runs the real diff engine
// over a heartbeat-interval change and feeds the resulting Change straight into
// buildSetConfigCommand — the exact path the 2PC dynamic-update loop takes.
//
// Before the fix, configdiff emitted Key="interval" (bare last segment), so the
// command was the invalid "set-config:context=network;interval=250" which
// Aerospike rejects; the dynamic update then always failed and the pod was
// cold-restarted even though network.heartbeat.interval is registered dynamic.
func TestDiffToSetConfigCommand_HeartbeatDynamic(t *testing.T) {
	oldCfg := map[string]any{
		"network": map[string]any{
			"heartbeat": map[string]any{"interval": 150},
			"fabric":    map[string]any{"send-threads": 4},
		},
	}
	newCfg := map[string]any{
		"network": map[string]any{
			"heartbeat": map[string]any{"interval": 250},
			"fabric":    map[string]any{"send-threads": 8},
		},
	}

	diff := configdiff.Diff(oldCfg, newCfg)
	if diff.HasStaticChanges() {
		t.Fatalf("heartbeat/fabric changes must be dynamic, got static: %+v", diff.Static)
	}
	if len(diff.Dynamic) != 2 {
		t.Fatalf("expected 2 dynamic changes, got %d (%+v)", len(diff.Dynamic), diff.Dynamic)
	}

	wantCmd := map[string]string{
		"network.heartbeat.interval":  "set-config:context=network;heartbeat.interval=250",
		"network.fabric.send-threads": "set-config:context=network;fabric.send-threads=8",
	}
	for _, change := range diff.Dynamic {
		cmd, err := buildSetConfigCommand(change)
		if err != nil {
			t.Fatalf("buildSetConfigCommand(%q) error = %v", change.Path, err)
		}
		want, ok := wantCmd[change.Path]
		if !ok {
			t.Errorf("unexpected change path %q", change.Path)
			continue
		}
		if cmd != want {
			t.Errorf("change %q: command = %q, want %q", change.Path, cmd, want)
		}
	}
}

func TestBuildSetConfigCommand_StringValue(t *testing.T) {
	change := configdiff.Change{
		Path:     "service.ticker-interval",
		Context:  "service",
		Key:      "ticker-interval",
		NewValue: "10",
	}

	cmd, err := buildSetConfigCommand(change)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "set-config:context=service;ticker-interval=10"
	if cmd != expected {
		t.Errorf("buildSetConfigCommand = %q, want %q", cmd, expected)
	}
}

func TestBuildSetConfigCommand_BoolValue(t *testing.T) {
	change := configdiff.Change{
		Path:      "namespace.read-page-cache",
		Context:   "namespace",
		Key:       "read-page-cache",
		NewValue:  true,
		Namespace: "testns",
	}

	cmd, err := buildSetConfigCommand(change)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "set-config:context=namespace;id=testns;read-page-cache=true"
	if cmd != expected {
		t.Errorf("buildSetConfigCommand = %q, want %q", cmd, expected)
	}
}

func TestBuildSetConfigCommand_NamespaceWithHighWaterDiskPct(t *testing.T) {
	change := configdiff.Change{
		Path:      "namespace.high-water-disk-pct",
		Context:   "namespace",
		Key:       "high-water-disk-pct",
		NewValue:  90,
		Namespace: "production",
	}

	cmd, err := buildSetConfigCommand(change)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "set-config:context=namespace;id=production;high-water-disk-pct=90"
	if cmd != expected {
		t.Errorf("buildSetConfigCommand = %q, want %q", cmd, expected)
	}
}

// --- validateDynamicChanges tests ---

func TestValidateDynamicChanges_AllValid(t *testing.T) {
	changes := []configdiff.Change{
		{Path: "service.proto-fd-max", Context: "service", Key: "proto-fd-max", NewValue: 20000},
		{Path: "namespace.default-ttl", Context: "namespace", Key: "default-ttl", NewValue: 3600, Namespace: "myns"},
	}
	if err := validateDynamicChanges(changes); err != nil {
		t.Errorf("expected nil error for all valid changes, got: %v", err)
	}
}

func TestValidateDynamicChanges_Empty(t *testing.T) {
	if err := validateDynamicChanges(nil); err != nil {
		t.Errorf("expected nil error for empty changes, got: %v", err)
	}
	if err := validateDynamicChanges([]configdiff.Change{}); err != nil {
		t.Errorf("expected nil error for empty slice, got: %v", err)
	}
}

func TestValidateDynamicChanges_OneInvalid(t *testing.T) {
	changes := []configdiff.Change{
		{Path: "service.proto-fd-max", Context: "service", Key: "proto-fd-max", NewValue: 20000},
		{Path: "service.bad", Context: "service", Key: "bad;key", NewValue: 100},
	}
	err := validateDynamicChanges(changes)
	if err == nil {
		t.Fatal("expected error for invalid change, got nil")
	}
	if !strings.Contains(err.Error(), "bad;key") {
		t.Errorf("error should mention the bad field, got: %v", err)
	}
	if !strings.Contains(err.Error(), "1 change(s)") {
		t.Errorf("error should report 1 failed change, got: %v", err)
	}
}

func TestValidateDynamicChanges_MultipleInvalid(t *testing.T) {
	changes := []configdiff.Change{
		{Path: "service.bad1", Context: "service", Key: "bad;key1", NewValue: 100},
		{Path: "service.ok", Context: "service", Key: "ok-key", NewValue: 200},
		{Path: "service.bad2", Context: "service;inject", Key: "good-key", NewValue: 300},
	}
	err := validateDynamicChanges(changes)
	if err == nil {
		t.Fatal("expected error for multiple invalid changes, got nil")
	}
	if !strings.Contains(err.Error(), "bad;key1") {
		t.Errorf("error should mention bad;key1, got: %v", err)
	}
	if !strings.Contains(err.Error(), "service;inject") {
		t.Errorf("error should mention service;inject, got: %v", err)
	}
	if !strings.Contains(err.Error(), "2 change(s)") {
		t.Errorf("error should report 2 failed changes, got: %v", err)
	}
}

// --- Input validation tests ---

func TestBuildSetConfigCommand_RejectsSemicolonInKey(t *testing.T) {
	change := configdiff.Change{
		Path:     "service.proto-fd-max",
		Context:  "service",
		Key:      "proto-fd-max;malicious=bad",
		NewValue: 20000,
	}

	_, err := buildSetConfigCommand(change)
	if err == nil {
		t.Error("expected error for semicolon in key")
	}
}

func TestBuildSetConfigCommand_RejectsSemicolonInNamespace(t *testing.T) {
	change := configdiff.Change{
		Path:      "namespace.default-ttl",
		Context:   "namespace",
		Key:       "default-ttl",
		NewValue:  3600,
		Namespace: "myns;malicious=bad",
	}

	_, err := buildSetConfigCommand(change)
	if err == nil {
		t.Error("expected error for semicolon in namespace")
	}
}

func TestBuildSetConfigCommand_RejectsColonInValue(t *testing.T) {
	change := configdiff.Change{
		Path:     "service.ticker-interval",
		Context:  "service",
		Key:      "ticker-interval",
		NewValue: "10:bad",
	}

	_, err := buildSetConfigCommand(change)
	if err == nil {
		t.Error("expected error for colon in value")
	}
}

func TestBuildSetConfigCommand_RejectsSemicolonInContext(t *testing.T) {
	change := configdiff.Change{
		Path:     "service.proto-fd-max",
		Context:  "service;inject",
		Key:      "proto-fd-max",
		NewValue: 20000,
	}

	_, err := buildSetConfigCommand(change)
	if err == nil {
		t.Error("expected error for semicolon in context")
	}
}

// TestBuildSetConfigCommand_RejectsUnsafeChars covers the hardened value
// validation: asinfo's wire format is also sensitive to '=', control
// characters (newline, carriage return, tab) and surrounding whitespace, any
// of which could inject a second directive.
func TestBuildSetConfigCommand_RejectsUnsafeChars(t *testing.T) {
	tests := []struct {
		name      string
		change    configdiff.Change
		wantInErr string
	}{
		{
			name: "equals in value",
			change: configdiff.Change{
				Context: "service", Key: "ticker-interval", NewValue: "10;migrate-threads=99",
			},
			wantInErr: "10;migrate-threads=99",
		},
		{
			name: "newline in value",
			change: configdiff.Change{
				Context: "service", Key: "ticker-interval", NewValue: "10\nset-config",
			},
			wantInErr: "control characters",
		},
		{
			name: "carriage return in value",
			change: configdiff.Change{
				Context: "service", Key: "ticker-interval", NewValue: "10\rinject",
			},
			wantInErr: "control characters",
		},
		{
			name: "tab in value",
			change: configdiff.Change{
				Context: "service", Key: "ticker-interval", NewValue: "10\tinject",
			},
			wantInErr: "control characters",
		},
		{
			name: "leading whitespace in value",
			change: configdiff.Change{
				Context: "service", Key: "ticker-interval", NewValue: " 10",
			},
			wantInErr: "whitespace",
		},
		{
			name: "trailing whitespace in value",
			change: configdiff.Change{
				Context: "service", Key: "ticker-interval", NewValue: "10 ",
			},
			wantInErr: "whitespace",
		},
		{
			name: "newline in namespace id",
			change: configdiff.Change{
				Context: "namespace", Key: "default-ttl", NewValue: 3600, Namespace: "myns\ninject",
			},
			wantInErr: "control characters",
		},
		{
			name: "equals in namespace id",
			change: configdiff.Change{
				Context: "namespace", Key: "default-ttl", NewValue: 3600, Namespace: "myns;id=other",
			},
			wantInErr: "myns;id=other",
		},
		{
			name: "control char in key",
			change: configdiff.Change{
				Context: "service", Key: "proto-fd-max\ninject", NewValue: 20000,
			},
			wantInErr: "proto-fd-max",
		},
		{
			name: "equals in key",
			change: configdiff.Change{
				Context: "service", Key: "proto-fd-max=evil", NewValue: 20000,
			},
			wantInErr: "proto-fd-max=evil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildSetConfigCommand(tt.change)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantInErr) {
				t.Errorf("error %q should mention %q", err.Error(), tt.wantInErr)
			}
		})
	}
}

// TestBuildSetConfigCommand_AllowsDottedKey ensures the hardened key validation
// still accepts legitimate dotted asinfo keys like heartbeat.interval.
func TestBuildSetConfigCommand_AllowsDottedKey(t *testing.T) {
	change := configdiff.Change{
		Path:     "network.heartbeat.interval",
		Context:  "network",
		Key:      "heartbeat.interval",
		NewValue: 250,
	}
	cmd, err := buildSetConfigCommand(change)
	if err != nil {
		t.Fatalf("unexpected error for dotted key: %v", err)
	}
	expected := "set-config:context=network;heartbeat.interval=250"
	if cmd != expected {
		t.Errorf("buildSetConfigCommand = %q, want %q", cmd, expected)
	}
}

// --- RollbackResult tests ---

func TestRollbackResult_HasFailures(t *testing.T) {
	tests := []struct {
		name     string
		result   RollbackResult
		expected bool
	}{
		{"no failures", RollbackResult{SuccessCount: 3, FailedCount: 0}, false},
		{"some failures", RollbackResult{SuccessCount: 2, FailedCount: 1, FailedPods: []string{"pod-1"}}, true},
		{"all failures", RollbackResult{SuccessCount: 0, FailedCount: 3, FailedPods: []string{"pod-0", "pod-1", "pod-2"}}, true},
		{"empty result", RollbackResult{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.HasFailures(); got != tt.expected {
				t.Errorf("RollbackResult.HasFailures() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// --- buildRollbackCommand tests ---

func TestBuildRollbackCommand_WithOldValue(t *testing.T) {
	log := logr.Discard()
	change := configdiff.Change{
		Path:     "service.proto-fd-max",
		Context:  "service",
		Key:      "proto-fd-max",
		OldValue: 15000,
		NewValue: 20000,
	}

	cmd := buildRollbackCommand(log, change)
	expected := "set-config:context=service;proto-fd-max=15000"
	if cmd != expected {
		t.Errorf("buildRollbackCommand = %q, want %q", cmd, expected)
	}
}

func TestBuildRollbackCommand_NilOldValue(t *testing.T) {
	log := logr.Discard()
	change := configdiff.Change{
		Path:     "service.proto-fd-max",
		Context:  "service",
		Key:      "proto-fd-max",
		OldValue: nil,
		NewValue: 20000,
	}

	cmd := buildRollbackCommand(log, change)
	if cmd != "" {
		t.Errorf("buildRollbackCommand should return empty for nil OldValue, got %q", cmd)
	}
}

func TestBuildRollbackCommand_InvalidOldValue(t *testing.T) {
	log := logr.Discard()
	change := configdiff.Change{
		Path:     "service.proto-fd-max",
		Context:  "service;inject",
		Key:      "proto-fd-max",
		OldValue: 15000,
		NewValue: 20000,
	}

	cmd := buildRollbackCommand(log, change)
	if cmd != "" {
		t.Errorf("buildRollbackCommand should return empty for invalid context, got %q", cmd)
	}
}

func TestBuildRollbackCommand_NamespaceScoped(t *testing.T) {
	log := logr.Discard()
	change := configdiff.Change{
		Path:      "namespace.default-ttl",
		Context:   "namespace",
		Key:       "default-ttl",
		OldValue:  1800,
		NewValue:  3600,
		Namespace: "myns",
	}

	cmd := buildRollbackCommand(log, change)
	expected := "set-config:context=namespace;id=myns;default-ttl=1800"
	if cmd != expected {
		t.Errorf("buildRollbackCommand = %q, want %q", cmd, expected)
	}
}

// --- perPodDynamicConfigTimeout tests ---

func TestPerPodDynamicConfigTimeout_Value(t *testing.T) {
	// Verify the timeout constant is reasonable for CE max (8 pods)
	// 30s × 8 = 240s < 300s (reconcileTimeout)
	maxPods := 8
	totalTime := perPodDynamicConfigTimeout * time.Duration(maxPods)
	if totalTime >= reconcileTimeout {
		t.Errorf("perPodDynamicConfigTimeout * maxPods (%v) should be less than reconcileTimeout (%v)",
			totalTime, reconcileTimeout)
	}
}
