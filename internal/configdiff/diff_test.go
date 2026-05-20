package configdiff

import (
	"testing"
)

func TestDiff_NilConfigs(t *testing.T) {
	result := Diff(nil, nil)
	if result.HasChanges() {
		t.Error("expected no changes for nil configs")
	}
}

func TestDiff_NoChanges(t *testing.T) {
	config := map[string]any{
		"service": map[string]any{
			"proto-fd-max": 15000,
		},
	}
	result := Diff(config, config)
	if result.HasChanges() {
		t.Errorf("expected no changes, got dynamic=%d static=%d", len(result.Dynamic), len(result.Static))
	}
}

func TestDiff_DynamicChange(t *testing.T) {
	old := map[string]any{
		"service": map[string]any{
			"proto-fd-max": 15000,
		},
	}
	new := map[string]any{
		"service": map[string]any{
			"proto-fd-max": 20000,
		},
	}

	result := Diff(old, new)

	if !result.HasChanges() {
		t.Fatal("expected changes")
	}
	if result.HasStaticChanges() {
		t.Error("expected no static changes")
	}
	if len(result.Dynamic) != 1 {
		t.Fatalf("expected 1 dynamic change, got %d", len(result.Dynamic))
	}
	if result.Dynamic[0].Path != "service.proto-fd-max" {
		t.Errorf("expected path service.proto-fd-max, got %s", result.Dynamic[0].Path)
	}
	if result.Dynamic[0].NewValue != 20000 {
		t.Errorf("expected new value 20000, got %v", result.Dynamic[0].NewValue)
	}
}

func TestDiff_StaticChange(t *testing.T) {
	old := map[string]any{
		"service": map[string]any{
			"cluster-name": "test",
		},
	}
	new := map[string]any{
		"service": map[string]any{
			"cluster-name": "test2",
		},
	}

	result := Diff(old, new)

	if !result.HasChanges() {
		t.Fatal("expected changes")
	}
	if !result.HasStaticChanges() {
		t.Error("expected static changes for cluster-name")
	}
	if len(result.Static) != 1 {
		t.Fatalf("expected 1 static change, got %d", len(result.Static))
	}
}

func TestDiff_MixedChanges(t *testing.T) {
	old := map[string]any{
		"service": map[string]any{
			"cluster-name": "test",
			"proto-fd-max": 15000,
		},
	}
	new := map[string]any{
		"service": map[string]any{
			"cluster-name": "test2",
			"proto-fd-max": 20000,
		},
	}

	result := Diff(old, new)

	if len(result.Dynamic) != 1 {
		t.Errorf("expected 1 dynamic change, got %d", len(result.Dynamic))
	}
	if len(result.Static) != 1 {
		t.Errorf("expected 1 static change, got %d", len(result.Static))
	}
}

func TestDiff_NewKeyAdded(t *testing.T) {
	old := map[string]any{
		"service": map[string]any{},
	}
	new := map[string]any{
		"service": map[string]any{
			"proto-fd-max": 20000,
		},
	}

	result := Diff(old, new)

	if len(result.Dynamic) != 1 {
		t.Fatalf("expected 1 dynamic change, got %d", len(result.Dynamic))
	}
	if result.Dynamic[0].OldValue != nil {
		t.Errorf("expected nil old value, got %v", result.Dynamic[0].OldValue)
	}
}

func TestDiff_KeyRemoved(t *testing.T) {
	old := map[string]any{
		"service": map[string]any{
			"proto-fd-max": 15000,
		},
	}
	new := map[string]any{
		"service": map[string]any{},
	}

	result := Diff(old, new)

	if !result.HasChanges() {
		t.Fatal("expected changes")
	}
}

func TestDiff_NamespaceChange(t *testing.T) {
	old := map[string]any{
		"namespaces": []any{
			map[string]any{
				"name":                "test",
				"default-ttl":         0,
				"high-water-disk-pct": 80,
			},
		},
	}
	new := map[string]any{
		"namespaces": []any{
			map[string]any{
				"name":                "test",
				"default-ttl":         3600,
				"high-water-disk-pct": 90,
			},
		},
	}

	result := Diff(old, new)

	if !result.HasChanges() {
		t.Fatal("expected changes")
	}
	// default-ttl and high-water-disk-pct are dynamic
	if len(result.Dynamic) != 2 {
		t.Errorf("expected 2 dynamic namespace changes, got %d", len(result.Dynamic))
	}
}

func TestDiff_NamespaceAdded(t *testing.T) {
	old := map[string]any{
		"namespaces": []any{},
	}
	new := map[string]any{
		"namespaces": []any{
			map[string]any{
				"name": "test",
			},
		},
	}

	result := Diff(old, new)

	if !result.HasStaticChanges() {
		t.Error("expected static change for namespace addition")
	}
}

func TestDiff_NamespaceRemoved(t *testing.T) {
	old := map[string]any{
		"namespaces": []any{
			map[string]any{"name": "ns1"},
			map[string]any{"name": "ns2"},
		},
	}
	new := map[string]any{
		"namespaces": []any{
			map[string]any{"name": "ns1"},
		},
	}

	result := Diff(old, new)

	if !result.HasStaticChanges() {
		t.Error("expected static change for namespace removal")
	}
}

func TestDiff_NamespaceKeyRemoved(t *testing.T) {
	old := map[string]any{
		"namespaces": []any{
			map[string]any{
				"name":        "test",
				"default-ttl": 3600,
			},
		},
	}
	new := map[string]any{
		"namespaces": []any{
			map[string]any{
				"name": "test",
			},
		},
	}

	result := Diff(old, new)

	if !result.HasChanges() {
		t.Fatal("expected changes when namespace key removed")
	}
}

func TestDiff_NestedSectionChange(t *testing.T) {
	old := map[string]any{
		"network": map[string]any{
			"heartbeat": map[string]any{
				"interval": 150,
				"timeout":  10,
			},
		},
	}
	new := map[string]any{
		"network": map[string]any{
			"heartbeat": map[string]any{
				"interval": 250,
				"timeout":  20,
			},
		},
	}

	result := Diff(old, new)

	if !result.HasChanges() {
		t.Fatal("expected changes")
	}
	// Both interval and timeout are dynamic
	if len(result.Dynamic) != 2 {
		t.Errorf("expected 2 dynamic changes, got %d", len(result.Dynamic))
	}
}

func TestDiff_NewTopLevelSection(t *testing.T) {
	old := map[string]any{
		"service": map[string]any{"proto-fd-max": 15000},
	}
	new := map[string]any{
		"service": map[string]any{"proto-fd-max": 15000},
		"logging": map[string]any{
			"any": "info",
		},
	}

	result := Diff(old, new)

	if !result.HasChanges() {
		t.Fatal("expected changes for new section")
	}
}

func TestDiff_EmptyOldConfig(t *testing.T) {
	old := map[string]any{}
	new := map[string]any{
		"service": map[string]any{
			"proto-fd-max": 15000,
		},
	}

	result := Diff(old, new)

	if !result.HasChanges() {
		t.Fatal("expected changes when adding to empty config")
	}
}

func TestDiffResult_HasChanges_EmptyResult(t *testing.T) {
	result := &DiffResult{}
	if result.HasChanges() {
		t.Error("empty DiffResult should not have changes")
	}
	if result.HasStaticChanges() {
		t.Error("empty DiffResult should not have static changes")
	}
}

func TestDiffResult_HasChanges_OnlyDynamic(t *testing.T) {
	result := &DiffResult{
		Dynamic: []Change{{Path: "service.proto-fd-max"}},
	}
	if !result.HasChanges() {
		t.Error("should have changes with dynamic changes")
	}
	if result.HasStaticChanges() {
		t.Error("should not have static changes")
	}
}

func TestDiffResult_HasStaticChanges_OnlyStatic(t *testing.T) {
	result := &DiffResult{
		Static: []Change{{Path: "service.cluster-name"}},
	}
	if !result.HasChanges() {
		t.Error("should have changes with static changes")
	}
	if !result.HasStaticChanges() {
		t.Error("should have static changes")
	}
}

func TestIsDynamic(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"service.proto-fd-max", true},
		{"service.cluster-name", false},
		{"namespace.default-ttl", true},
		{"namespace.high-water-disk-pct", true},
		{"namespace.high-water-memory-pct", true},
		{"namespace.stop-writes-pct", true},
		// memory-size and replication-factor are NOT dynamic — applying them
		// via set-config can cause replicas to drift / data loss; they require
		// a synchronized cold restart and must fall through to that path.
		{"namespace.memory-size", false},
		{"namespace.replication-factor", false},
		{"network.heartbeat.interval", true},
		{"network.heartbeat.timeout", true},
		{"network.service.port", false},
		{"logging.any", true},
		{"logging.misc", true},
		{"service.migrate-threads", true},
		{"service.batch-index-threads", true},
		{"security.log.report-authentication", true},
		{"unknown.param", false},
		{"", false},
	}

	for _, tc := range tests {
		if got := IsDynamic(tc.path); got != tc.expected {
			t.Errorf("IsDynamic(%q) = %v, expected %v", tc.path, got, tc.expected)
		}
	}
}

// --- Helper function tests ---

func TestJoinPath_EmptyPrefix(t *testing.T) {
	if got := joinPath("", "key"); got != "key" {
		t.Errorf("joinPath(\"\", \"key\") = %q, want \"key\"", got)
	}
}

func TestJoinPath_WithPrefix(t *testing.T) {
	if got := joinPath("service", "proto-fd-max"); got != "service.proto-fd-max" {
		t.Errorf("joinPath(\"service\", \"proto-fd-max\") = %q, want \"service.proto-fd-max\"", got)
	}
}

func TestFirstSegment(t *testing.T) {
	tests := []struct {
		path, expected string
	}{
		{"service.proto-fd-max", "service"},
		{"network.heartbeat.interval", "network"},
		{"single", "single"},
	}
	for _, tc := range tests {
		if got := firstSegment(tc.path); got != tc.expected {
			t.Errorf("firstSegment(%q) = %q, want %q", tc.path, got, tc.expected)
		}
	}
}

func TestLastSegment(t *testing.T) {
	tests := []struct {
		path, expected string
	}{
		{"service.proto-fd-max", "proto-fd-max"},
		{"network.heartbeat.interval", "interval"},
		{"single", "single"},
	}
	for _, tc := range tests {
		if got := lastSegment(tc.path); got != tc.expected {
			t.Errorf("lastSegment(%q) = %q, want %q", tc.path, got, tc.expected)
		}
	}
}

func TestValuesEqual(t *testing.T) {
	tests := []struct {
		a, b     any
		expected bool
	}{
		{1, 1, true},
		{1, 2, false},
		{"foo", "foo", true},
		{"foo", "bar", false},
		{true, true, true},
		{true, false, false},
		{nil, nil, true},
		// Cross-type numeric equality: a value decoded as int from a Go
		// literal must equal the same value decoded as float64 after a
		// JSON round-trip, otherwise the diff reports a phantom change.
		{15000, float64(15000), true},
		{float64(15000), 15000, true},
		{int64(60), float64(60), true},
		{int64(60), 60, true},
		{float64(2), int32(2), true},
		{1, float64(2), false},
		{int64(15000), float64(15001), false},
		// A number and a non-number are never equal.
		{1, "1", false},
		{0, false, false},
	}
	for _, tc := range tests {
		if got := valuesEqual(tc.a, tc.b); got != tc.expected {
			t.Errorf("valuesEqual(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.expected)
		}
	}
}

func TestAsSlice_Nil(t *testing.T) {
	if got := asSlice(nil); got != nil {
		t.Errorf("asSlice(nil) = %v, want nil", got)
	}
}

func TestAsSlice_ValidSlice(t *testing.T) {
	input := []any{"a", "b"}
	got := asSlice(input)
	if len(got) != 2 {
		t.Errorf("asSlice([]any) len = %d, want 2", len(got))
	}
}

func TestAsSlice_NonSlice(t *testing.T) {
	if got := asSlice("not a slice"); got != nil {
		t.Errorf("asSlice(string) = %v, want nil", got)
	}
}
