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
