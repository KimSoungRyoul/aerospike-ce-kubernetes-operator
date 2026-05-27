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

// Package telemetry wires the operator's OpenTelemetry pipeline — traces,
// metrics, and logs exported to an OTLP/gRPC collector.
//
// It is OFF by default. Setup is a no-op unless OTEL_SDK_DISABLED is
// explicitly set to a falsy value (e.g. "false"); while off, the global OTel
// providers stay NoOp and instrumentation costs effectively nothing. The
// unset-means-disabled default matches the aerospike-cluster-manager API so
// both halves of the stack behave the same way.
//
// When on, every exporter / sampler / resource detail is taken from the
// OpenTelemetry SDK standard environment variables — OTEL_EXPORTER_OTLP_ENDPOINT,
// OTEL_EXPORTER_OTLP_HEADERS, OTEL_TRACES_SAMPLER, OTEL_RESOURCE_ATTRIBUTES,
// OTEL_SERVICE_NAME, … — so no operator-specific wrapper flags are introduced.
// Signals exported:
//
//   - traces  : reconcile spans (see internal/controller).
//   - metrics : the whole controller-runtime + ACKO Prometheus registry,
//     bridged to OTLP — so the exact same series stay available both via the
//     /metrics scrape endpoint and as an OTLP push, with no metric-definition
//     code changes.
//   - logs    : the operator's zap log stream, teed into the OTLP log pipeline
//     by the otelzap bridge (wired in cmd/main.go via the core from ZapCore).
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"go.opentelemetry.io/contrib/bridges/otelzap"
	prombridge "go.opentelemetry.io/contrib/bridges/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otlplog "go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	otlpmetric "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	otlptrace "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	logglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap/zapcore"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// defaultAttributeValueLengthLimit caps the bytes any single span / log
// attribute value can grow to before the SDK truncates it. The OTel Go SDK
// default is -1 (unlimited), which is unsafe for an operator that watches
// cluster-wide: if any reconcile-path log or span ever carries a serialized
// Kubernetes object as an attribute (Pod / StatefulSet / Service / Secret),
// a single OTLP/gRPC export can balloon past 4 MiB and trigger OOMKilled
// in busy clusters (#299). 4 KiB is enough to keep human-readable identifiers
// and short error strings intact; anything larger is truncated by the SDK
// with a deterministic suffix marker, which is the right trade-off for an
// operator's observability budget.
//
// Operators who need a higher cap can override via the standard spec env
// vars OTEL_ATTRIBUTE_VALUE_LENGTH_LIMIT / OTEL_SPAN_ATTRIBUTE_VALUE_LENGTH_LIMIT
// (and the OTEL_LOGRECORD_* counterparts) — see resolveAttributeValueLengthLimit.
const defaultAttributeValueLengthLimit = 4096

// ServiceName is the OTel service.name stamped on every signal the operator
// emits, and the instrumentation-scope name used for its tracer.
const ServiceName = "aerospike-ce-kubernetes-operator"

// Provider holds the operator's configured OTel SDK providers together with
// the hooks needed to flush and shut them down. A disabled Provider (the
// default) is fully inert: Enabled() is false, ZapCore() is nil, and
// Shutdown() is a no-op.
type Provider struct {
	enabled       bool
	zapCore       zapcore.Core
	shutdownFuncs []func(context.Context) error
}

// Enabled reports whether the OTel pipeline was activated.
func (p *Provider) Enabled() bool { return p != nil && p.enabled }

// ZapCore returns a zapcore.Core that mirrors the operator's log stream into
// the OTLP log pipeline, or nil when telemetry is disabled. cmd/main.go tees
// this into the controller-runtime zap logger via zap.WrapCore.
func (p *Provider) ZapCore() zapcore.Core {
	if p == nil {
		return nil
	}
	return p.zapCore
}

// Shutdown flushes and stops every provider, giving the batch processors a
// final chance to export. Safe to call on a nil or disabled Provider, and
// safe to call after Setup already returned an error (it is then a no-op).
func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil {
		return nil
	}
	var errs []error
	// Shut down in reverse registration order (logs, then metrics, then traces).
	for i := len(p.shutdownFuncs) - 1; i >= 0; i-- {
		if err := p.shutdownFuncs[i](ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// activated reports whether OTEL_SDK_DISABLED opts the operator INTO
// telemetry. The operator ships with telemetry off, so an unset variable
// means disabled.
func activated() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_SDK_DISABLED")))
	if v == "" {
		return false
	}
	return v != "true" && v != "1" && v != "yes"
}

// signalEnabled reports whether the named signal (traces / metrics / logs)
// should be exported. Returns:
//
//   - (true,  nil)  for "otlp" or unset — the spec default
//   - (false, nil)  for "none"          — caller skips provider construction
//   - (false, err)  for any other value — typo in chart values, surface loudly
//
// This honors the standard OTel SDK env vars OTEL_TRACES_EXPORTER /
// OTEL_METRICS_EXPORTER / OTEL_LOGS_EXPORTER so operators can disable a single
// signal (e.g. metrics) when their collector ships traces+logs receivers but
// no metrics pipeline — without having to disable telemetry wholesale via
// OTEL_SDK_DISABLED=true. Same workaround channel used by #299 reporters to
// stop the OOM cycle while keeping logs flowing through fluent-bit.
//
// Reference:
// https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/#exporter-selection
func signalEnabled(signal string) (bool, error) {
	envKey := "OTEL_" + strings.ToUpper(signal) + "_EXPORTER"
	v := strings.ToLower(strings.TrimSpace(os.Getenv(envKey)))
	switch v {
	case "", "otlp":
		return true, nil
	case "none":
		return false, nil
	default:
		return false, fmt.Errorf("unsupported %s=%q; expected 'otlp' or 'none'", envKey, v)
	}
}

// resolveAttributeValueLengthLimit picks the effective per-attribute byte cap.
//
// The OTel Go SDK's sdktrace.NewSpanLimits() already reads
// OTEL_ATTRIBUTE_VALUE_LENGTH_LIMIT / OTEL_SPAN_ATTRIBUTE_VALUE_LENGTH_LIMIT
// and applies them, but the SDK's hard-coded default when both are unset is
// -1 (unlimited). For an operator that watches cluster-wide, unlimited means
// a single zap field carrying a serialized Pod / StatefulSet body can put the
// whole OTLP export over the receiver's 4 MiB limit, so we substitute a 4 KiB
// cap whenever the env vars do not pin a value explicitly.
//
// `specific` is the signal-scoped key (e.g. OTEL_SPAN_ATTRIBUTE_VALUE_LENGTH_LIMIT
// for spans, OTEL_LOGRECORD_ATTRIBUTE_VALUE_LENGTH_LIMIT for logs); `general`
// is the cross-signal fallback (OTEL_ATTRIBUTE_VALUE_LENGTH_LIMIT). The
// specific key takes precedence so a "tighter logs but laxer traces"
// configuration is reachable.
func resolveAttributeValueLengthLimit(specific, general string) int {
	for _, key := range []string{specific, general} {
		if raw, ok := os.LookupEnv(key); ok {
			if n, err := parseIntStrict(raw); err == nil && n >= 0 {
				return n
			}
		}
	}
	return defaultAttributeValueLengthLimit
}

func parseIntStrict(s string) (int, error) {
	s = strings.TrimSpace(s)
	n := 0
	for i, ch := range s {
		if i == 0 && ch == '-' {
			return 0, fmt.Errorf("negative limit not allowed")
		}
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("non-digit %q", ch)
		}
		n = n*10 + int(ch-'0')
	}
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	return n, nil
}

// Setup builds the OTLP trace/metric/log providers, registers them as the
// global OpenTelemetry providers, and installs the W3C trace-context + baggage
// propagator. serviceVersion is stamped onto the resource as service.version.
//
// When OTEL_SDK_DISABLED keeps telemetry off, Setup returns a disabled
// Provider and no error. Per-signal disable (OTEL_TRACES_EXPORTER=none /
// OTEL_METRICS_EXPORTER=none / OTEL_LOGS_EXPORTER=none) is honored: the
// matching provider is left as the global NoOp default while the remaining
// signals still export. When a provider fails to build, any partially
// constructed providers are flushed and stopped, an inert Provider is
// returned, and the error is surfaced so the caller can keep running the
// operator with telemetry off.
func Setup(ctx context.Context, serviceVersion string) (*Provider, error) {
	if !activated() {
		return &Provider{}, nil
	}

	tracesOn, err := signalEnabled("traces")
	if err != nil {
		return &Provider{}, err
	}
	metricsOn, err := signalEnabled("metrics")
	if err != nil {
		return &Provider{}, err
	}
	logsOn, err := signalEnabled("logs")
	if err != nil {
		return &Provider{}, err
	}

	p := &Provider{}
	// fail flushes whatever was built so far and returns an inert Provider,
	// so the caller's deferred Shutdown becomes a clean no-op.
	fail := func(err error) (*Provider, error) {
		_ = p.Shutdown(ctx)
		return &Provider{}, err
	}

	// Defaults first, then WithFromEnv so OTEL_SERVICE_NAME /
	// OTEL_RESOURCE_ATTRIBUTES can override service.name / extra attributes.
	res, err := resource.New(ctx,
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			attribute.String("service.name", ServiceName),
			attribute.String("service.version", serviceVersion),
		),
		resource.WithFromEnv(),
	)
	if err != nil {
		return fail(err)
	}

	// Build all three exporters/providers BEFORE touching any global state.
	// If a later exporter fails, fail() shuts down whatever was built and the
	// global OTel providers are never mutated — they stay NoOp, so the
	// operator continues cleanly with telemetry off.

	// --- Traces ---------------------------------------------------------
	var tp *sdktrace.TracerProvider
	if tracesOn {
		traceExp, terr := otlptrace.New(ctx)
		if terr != nil {
			return fail(terr)
		}
		// SpanLimits.AttributeValueLengthLimit defaults to -1 (unlimited) in
		// the SDK. That is unsafe for a cluster-wide watcher because any zap
		// field or span attribute carrying a serialized Kubernetes object
		// would let a single OTLP export grow past the 4 MiB collector default
		// and crash the operator with OOMKilled (#299). Cap the length and
		// keep all other count limits at their 128 SDK defaults.
		spanLimits := sdktrace.NewSpanLimits()
		spanLimits.AttributeValueLengthLimit = resolveAttributeValueLengthLimit(
			"OTEL_SPAN_ATTRIBUTE_VALUE_LENGTH_LIMIT",
			"OTEL_ATTRIBUTE_VALUE_LENGTH_LIMIT",
		)
		tp = sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(traceExp),
			sdktrace.WithResource(res),
			sdktrace.WithRawSpanLimits(spanLimits),
		)
		p.shutdownFuncs = append(p.shutdownFuncs, tp.Shutdown)
	}

	// --- Metrics --------------------------------------------------------
	// The Prometheus bridge republishes the controller-runtime metrics
	// registry — the built-in reconcile/workqueue metrics plus every ACKO
	// custom metric from internal/metrics — through OTLP. No metric
	// definition changes: the same series stay scrapable at /metrics and
	// are additionally pushed to the collector.
	var mp *sdkmetric.MeterProvider
	if metricsOn {
		metricExp, merr := otlpmetric.New(ctx)
		if merr != nil {
			return fail(merr)
		}
		reader := sdkmetric.NewPeriodicReader(metricExp,
			sdkmetric.WithProducer(prombridge.NewMetricProducer(
				prombridge.WithGatherer(ctrlmetrics.Registry),
			)),
		)
		mp = sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(reader),
			sdkmetric.WithResource(res),
		)
		p.shutdownFuncs = append(p.shutdownFuncs, mp.Shutdown)
	}

	// --- Logs -----------------------------------------------------------
	var lp *sdklog.LoggerProvider
	if logsOn {
		logExp, lerr := otlplog.New(ctx)
		if lerr != nil {
			return fail(lerr)
		}
		logAttrValLimit := resolveAttributeValueLengthLimit(
			"OTEL_LOGRECORD_ATTRIBUTE_VALUE_LENGTH_LIMIT",
			"OTEL_ATTRIBUTE_VALUE_LENGTH_LIMIT",
		)
		lp = sdklog.NewLoggerProvider(
			sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
			sdklog.WithResource(res),
			// Cap per-log-record attribute byte size for the same reason as
			// SpanLimits above. Zap fields that wrap a Pod/StatefulSet via
			// otelzap would otherwise produce multi-megabyte LogRecords on
			// every reconcile in a cluster-wide watch (#299).
			sdklog.WithAttributeValueLengthLimit(logAttrValLimit),
		)
		p.shutdownFuncs = append(p.shutdownFuncs, lp.Shutdown)
	}

	// All providers built successfully — now publish them globally. Skip the
	// SetXProvider call for any disabled signal so the global NoOp survives.
	if tp != nil {
		otel.SetTracerProvider(tp)
	}
	if mp != nil {
		otel.SetMeterProvider(mp)
	}
	if lp != nil {
		logglobal.SetLoggerProvider(lp)
		// otelzap core — cmd/main.go tees this into the zap logger so operator
		// log lines are exported as OTLP log records alongside traces. Only
		// attach the core when logs are actually enabled; otherwise leave
		// p.zapCore nil so the zap tee is skipped.
		p.zapCore = otelzap.NewCore(ServiceName, otelzap.WithLoggerProvider(lp))
	}
	// W3C TraceContext + Baggage so operator spans stitch with the
	// cluster-manager API and any other OTel-instrumented service.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Enabled reflects whether ANY signal is being exported. All-three-off via
	// per-signal flags is a legitimate no-op configuration — the caller will
	// log "OpenTelemetry export enabled" only when at least one pipeline runs.
	p.enabled = tp != nil || mp != nil || lp != nil
	return p, nil
}
