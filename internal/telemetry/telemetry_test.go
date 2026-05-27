/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package telemetry

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestActivated(t *testing.T) {
	cases := []struct {
		name string
		val  string
		want bool
	}{
		{name: "empty disables", val: "", want: false},
		{name: "true disables", val: "true", want: false},
		{name: "TRUE disables", val: "TRUE", want: false},
		{name: "1 disables", val: "1", want: false},
		{name: "yes disables", val: "yes", want: false},
		{name: "false enables", val: "false", want: true},
		{name: "0 enables", val: "0", want: true},
		{name: "padded false enables", val: "  false  ", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OTEL_SDK_DISABLED", tc.val)
			if got := activated(); got != tc.want {
				t.Errorf("activated() with OTEL_SDK_DISABLED=%q = %v, want %v", tc.val, got, tc.want)
			}
		})
	}
}

func TestSetupDisabledByDefault(t *testing.T) {
	// An empty OTEL_SDK_DISABLED behaves like an unset one: telemetry off.
	t.Setenv("OTEL_SDK_DISABLED", "")

	p, err := Setup(context.Background(), "test")
	if err != nil {
		t.Fatalf("Setup() error = %v, want nil", err)
	}
	if p.Enabled() {
		t.Error("Enabled() = true, want false when telemetry is off")
	}
	if p.ZapCore() != nil {
		t.Error("ZapCore() != nil on a disabled provider")
	}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown() on a disabled provider = %v, want nil", err)
	}
}

func TestSetupDisabledExplicit(t *testing.T) {
	t.Setenv("OTEL_SDK_DISABLED", "true")

	p, err := Setup(context.Background(), "test")
	if err != nil {
		t.Fatalf("Setup() error = %v, want nil", err)
	}
	if p.Enabled() {
		t.Error("Enabled() = true, want false when OTEL_SDK_DISABLED=true")
	}
}

func TestSetupEnabled(t *testing.T) {
	t.Setenv("OTEL_SDK_DISABLED", "false")
	// A bogus endpoint is fine: the OTLP/gRPC exporters connect lazily, so
	// Setup succeeds without a reachable collector.
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")

	p, err := Setup(context.Background(), "v9.9.9-test")
	if err != nil {
		t.Fatalf("Setup() error = %v, want nil", err)
	}
	if !p.Enabled() {
		t.Fatal("Enabled() = false, want true when OTEL_SDK_DISABLED=false")
	}
	if p.ZapCore() == nil {
		t.Error("ZapCore() = nil on an enabled provider, want a non-nil otelzap core")
	}

	// Shutdown of an idle pipeline must return within the deadline even with
	// no collector listening (the metric reader's final export simply fails
	// fast against the unreachable endpoint).
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = p.Shutdown(ctx)
}

func TestProviderNilSafe(t *testing.T) {
	var p *Provider
	if p.Enabled() {
		t.Error("nil Provider Enabled() = true, want false")
	}
	if p.ZapCore() != nil {
		t.Error("nil Provider ZapCore() != nil")
	}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Errorf("nil Provider Shutdown() = %v, want nil", err)
	}
}

// ---------------------------------------------------------------------------
// Per-signal exporter selection (#299)
// ---------------------------------------------------------------------------

func TestSignalEnabled(t *testing.T) {
	cases := []struct {
		name      string
		signal    string
		val       string
		want      bool
		wantError bool
	}{
		{name: "unset traces -> enabled", signal: "traces", val: "", want: true},
		{name: "otlp metrics -> enabled", signal: "metrics", val: "otlp", want: true},
		{name: "OTLP uppercase -> enabled", signal: "logs", val: "OTLP", want: true},
		{name: "padded otlp -> enabled", signal: "logs", val: "  otlp  ", want: true},
		{name: "none traces -> disabled", signal: "traces", val: "none", want: false},
		{name: "NONE metrics -> disabled", signal: "metrics", val: "NONE", want: false},
		{name: "padded none -> disabled", signal: "logs", val: " none ", want: false},
		{name: "jaeger traces -> error", signal: "traces", val: "jaeger", wantError: true},
		{name: "console logs -> error", signal: "logs", val: "console", wantError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OTEL_"+strings.ToUpper(tc.signal)+"_EXPORTER", tc.val)
			got, err := signalEnabled(tc.signal)
			if (err != nil) != tc.wantError {
				t.Fatalf("signalEnabled(%q) err = %v, wantError %v", tc.signal, err, tc.wantError)
			}
			if got != tc.want {
				t.Errorf("signalEnabled(%q)=%v, want %v", tc.signal, got, tc.want)
			}
		})
	}
}

func TestSetupTracesDisabled(t *testing.T) {
	// OTEL_LOGS_EXPORTER=none must keep metrics+logs running but suppress the
	// trace pipeline. zapCore must be present because logs are still on; the
	// global TracerProvider is left as the NoOp default.
	t.Setenv("OTEL_SDK_DISABLED", "false")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
	t.Setenv("OTEL_TRACES_EXPORTER", "none")

	p, err := Setup(context.Background(), "test")
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	if !p.Enabled() {
		t.Fatal("Enabled()=false, want true (logs and metrics are still on)")
	}
	if p.ZapCore() == nil {
		t.Error("ZapCore()=nil, want non-nil when logs are on")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = p.Shutdown(ctx)
}

func TestSetupMetricsDisabled(t *testing.T) {
	t.Setenv("OTEL_SDK_DISABLED", "false")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")

	p, err := Setup(context.Background(), "test")
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	if !p.Enabled() {
		t.Fatal("Enabled()=false, want true (traces and logs still on)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = p.Shutdown(ctx)
}

func TestSetupLogsDisabled(t *testing.T) {
	t.Setenv("OTEL_SDK_DISABLED", "false")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
	t.Setenv("OTEL_LOGS_EXPORTER", "none")

	p, err := Setup(context.Background(), "test")
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	if !p.Enabled() {
		t.Fatal("Enabled()=false, want true (traces and metrics still on)")
	}
	if p.ZapCore() != nil {
		t.Error("ZapCore()!=nil when logs are disabled; would still tee log records into the OTLP pipeline")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = p.Shutdown(ctx)
}

func TestSetupAllSignalsDisabled(t *testing.T) {
	// Disabling every signal individually is a legitimate no-op configuration.
	// Setup must not crash and must report Enabled()=false so the caller's
	// "OpenTelemetry export enabled" log is suppressed.
	t.Setenv("OTEL_SDK_DISABLED", "false")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	t.Setenv("OTEL_LOGS_EXPORTER", "none")

	p, err := Setup(context.Background(), "test")
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	if p.Enabled() {
		t.Error("Enabled()=true when every signal is disabled")
	}
	if p.ZapCore() != nil {
		t.Error("ZapCore()!=nil when every signal is disabled")
	}
}

func TestSetupRejectsUnknownExporter(t *testing.T) {
	// A typo in chart values (e.g. OTEL_TRACES_EXPORTER=jaeger) must surface
	// as a startup error rather than silently disabling the signal.
	t.Setenv("OTEL_SDK_DISABLED", "false")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
	t.Setenv("OTEL_TRACES_EXPORTER", "jaeger")

	p, err := Setup(context.Background(), "test")
	if err == nil {
		t.Fatal("Setup() error = nil, want error for unsupported OTEL_TRACES_EXPORTER")
	}
	if p.Enabled() {
		t.Error("Enabled()=true on a failed Setup")
	}
}

// ---------------------------------------------------------------------------
// Attribute-value length caps (#299 OOM root cause)
// ---------------------------------------------------------------------------

func TestResolveLogAttributeValueLengthLimitDefault(t *testing.T) {
	t.Setenv("OTEL_LOGRECORD_ATTRIBUTE_VALUE_LENGTH_LIMIT", "")
	t.Setenv("OTEL_ATTRIBUTE_VALUE_LENGTH_LIMIT", "")

	got := resolveLogAttributeValueLengthLimit()
	if got != defaultAttributeValueLengthLimit {
		t.Errorf("resolveLogAttributeValueLengthLimit() = %d, want %d", got, defaultAttributeValueLengthLimit)
	}
}

func TestResolveLogAttributeValueLengthLimitSpecificOverridesGeneral(t *testing.T) {
	t.Setenv("OTEL_ATTRIBUTE_VALUE_LENGTH_LIMIT", "1024")
	t.Setenv("OTEL_LOGRECORD_ATTRIBUTE_VALUE_LENGTH_LIMIT", "8192")

	got := resolveLogAttributeValueLengthLimit()
	if got != 8192 {
		t.Errorf("logrecord-specific=8192 should win over general=1024, got %d", got)
	}
}

func TestResolveLogAttributeValueLengthLimitGeneralFallback(t *testing.T) {
	t.Setenv("OTEL_LOGRECORD_ATTRIBUTE_VALUE_LENGTH_LIMIT", "")
	t.Setenv("OTEL_ATTRIBUTE_VALUE_LENGTH_LIMIT", "2048")

	got := resolveLogAttributeValueLengthLimit()
	if got != 2048 {
		t.Errorf("general=2048 should be used when logrecord-specific is unset, got %d", got)
	}
}

func TestResolveLogAttributeValueLengthLimitMalformedFallsBackToDefault(t *testing.T) {
	// Malformed values must not silently revert to SDK unlimited (-1); we
	// pretend the env var was not set and apply the safe default cap.
	t.Setenv("OTEL_LOGRECORD_ATTRIBUTE_VALUE_LENGTH_LIMIT", "abc")
	t.Setenv("OTEL_ATTRIBUTE_VALUE_LENGTH_LIMIT", "")

	got := resolveLogAttributeValueLengthLimit()
	if got != defaultAttributeValueLengthLimit {
		t.Errorf("malformed value should fall back to %d, got %d", defaultAttributeValueLengthLimit, got)
	}
}

func TestResolveLogAttributeValueLengthLimitNegativeFallsBackToDefault(t *testing.T) {
	// A negative override would mean "unlimited" per the OTel SDK, which is
	// exactly the unsafe configuration #299 hit. Force the safe default
	// instead of trusting the user-supplied -1.
	t.Setenv("OTEL_LOGRECORD_ATTRIBUTE_VALUE_LENGTH_LIMIT", "-1")
	t.Setenv("OTEL_ATTRIBUTE_VALUE_LENGTH_LIMIT", "")

	got := resolveLogAttributeValueLengthLimit()
	if got != defaultAttributeValueLengthLimit {
		t.Errorf("negative override should fall back to %d, got %d", defaultAttributeValueLengthLimit, got)
	}
}

func TestDefaultAttributeValueLengthLimitIsCappedForOOMProtection(t *testing.T) {
	// Sanity guard: the constant must remain a finite, OTLP-receiver-safe cap.
	// Anyone bumping it past the 4 MiB gRPC default needs to revisit #299 and
	// coordinate with the collector configuration.
	if defaultAttributeValueLengthLimit <= 0 {
		t.Fatalf("defaultAttributeValueLengthLimit must be a positive byte cap, got %d",
			defaultAttributeValueLengthLimit)
	}
	if defaultAttributeValueLengthLimit > 64*1024 {
		t.Fatalf("defaultAttributeValueLengthLimit=%d > 64 KiB; revisit #299 before raising",
			defaultAttributeValueLengthLimit)
	}
}
