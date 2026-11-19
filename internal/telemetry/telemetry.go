// Package telemetry sets up OpenTelemetry traces and metrics for the Athena
// server. It is off unless an OTLP endpoint is configured with the standard
// OTEL_EXPORTER_OTLP_ENDPOINT (or the traces/metrics specific) variables, so
// no collector is required to run Athena.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// Telemetry holds the configured providers. A zero Telemetry is disabled.
type Telemetry struct {
	enabled  bool
	endpoint *url.URL
	shutdown []func(context.Context) error
}

// Enabled reports whether traces and metrics are being exported.
func (t *Telemetry) Enabled() bool { return t != nil && t.enabled }

// endpointFromEnv returns the OTLP/HTTP base endpoint, or nil when telemetry is off.
func endpointFromEnv() (*url.URL, error) {
	raw := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if raw == "" {
		return nil, nil
	}
	endpoint, err := url.Parse(raw)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" {
		return nil, fmt.Errorf("OTEL_EXPORTER_OTLP_ENDPOINT must be an http(s) URL such as http://127.0.0.1:4318, got %q", raw)
	}
	return endpoint, nil
}

// Setup installs global tracer and meter providers when an endpoint is set.
// attrs describe this process, for example the project slug.
func Setup(ctx context.Context, version string, attrs ...attribute.KeyValue) (*Telemetry, error) {
	endpoint, err := endpointFromEnv()
	if err != nil || endpoint == nil {
		return &Telemetry{}, err
	}
	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(semconv.SchemaURL,
		append([]attribute.KeyValue{semconv.ServiceName(serviceName()), semconv.ServiceVersion(version)}, attrs...)...))
	if err != nil {
		return nil, fmt.Errorf("telemetry resource: %w", err)
	}
	traceExporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("trace exporter: %w", err)
	}
	metricExporter, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("metric exporter: %w", err)
	}
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(traceExporter), sdktrace.WithResource(res))
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter, sdkmetric.WithInterval(15*time.Second))), sdkmetric.WithResource(res))
	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	otel.SetErrorHandler(reportError)
	log.Printf("OpenTelemetry exporting traces and metrics to %s", endpoint.Redacted())
	return &Telemetry{
		enabled:  true,
		endpoint: endpoint,
		shutdown: []func(context.Context) error{tracerProvider.Shutdown, meterProvider.Shutdown},
	}, nil
}

// Shutdown flushes pending telemetry.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	if t == nil {
		return nil
	}
	var errs []error
	for _, fn := range t.shutdown {
		errs = append(errs, fn(ctx))
	}
	return errors.Join(errs...)
}

// BrowserTracesHandler forwards OTLP/HTTP trace exports from the desk to the
// collector, so the browser posts to its own origin and never needs the
// collector's address. Only POST /otlp/v1/traces is forwarded, bodies are
// capped, and no other collector path is reachable.
func (t *Telemetry) BrowserTracesHandler() http.Handler {
	if !t.Enabled() {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "telemetry is not enabled", http.StatusNotFound)
		})
	}
	target := *t.endpoint
	target.Path = strings.TrimSuffix(target.Path, "/") + "/v1/traces"
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.Out.URL = &target
			r.Out.Host = target.Host
			r.Out.Header.Del("Cookie")
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			reportError(fmt.Errorf("forward browser traces: %w", err))
			w.WriteHeader(http.StatusBadGateway)
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		proxy.ServeHTTP(w, r)
	})
}

func serviceName() string {
	if name := os.Getenv("OTEL_SERVICE_NAME"); name != "" {
		return name
	}
	return "athena"
}

// reportError logs exporter errors at most once a minute, so a stopped
// collector does not flood the server log.
var reportError = func() otel.ErrorHandlerFunc {
	var mu sync.Mutex
	var last time.Time
	return func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if time.Since(last) < time.Minute {
			return
		}
		last = time.Now()
		log.Printf("telemetry: %v (repeats suppressed for a minute)", err)
	}
}()
