import { context, propagation, trace } from '@opentelemetry/api'
import { W3CTraceContextPropagator } from '@opentelemetry/core'
import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-http'
import { registerInstrumentations } from '@opentelemetry/instrumentation'
import { DocumentLoadInstrumentation } from '@opentelemetry/instrumentation-document-load'
import { FetchInstrumentation } from '@opentelemetry/instrumentation-fetch'
import { resourceFromAttributes } from '@opentelemetry/resources'
import { BatchSpanProcessor, StackContextManager, TracerProvider } from '@opentelemetry/sdk-trace'
import { ATTR_SERVICE_NAME, ATTR_SERVICE_VERSION } from '@opentelemetry/semantic-conventions'

let started = false

/**
 * Starts browser tracing when the server reports telemetry is on. Spans go to
 * /otlp/v1/traces on this origin; the Go server forwards them to the collector.
 * Fetch spans carry a traceparent header, so ConnectRPC calls continue as
 * server spans in the same trace. Request and response bodies are never recorded.
 */
export function startBrowserTelemetry(version: string, projectSlug: string) {
  if (started) return
  started = true
  const provider = new TracerProvider({
    resource: resourceFromAttributes({
      [ATTR_SERVICE_NAME]: 'athena-desk',
      [ATTR_SERVICE_VERSION]: version,
      ...(projectSlug ? { 'athena.project.slug': projectSlug } : {}),
    }),
    spanProcessors: [new BatchSpanProcessor({ exporter: new OTLPTraceExporter({ url: `${window.location.origin}/otlp/v1/traces` }) })],
  })
  trace.setGlobalTracerProvider(provider)
  // Fetch instrumentation injects traceparent from the active context while it
  // starts the request; a stack context manager is enough for that synchronous step.
  context.setGlobalContextManager(new StackContextManager().enable())
  propagation.setGlobalPropagator(new W3CTraceContextPropagator())
  registerInstrumentations({
    instrumentations: [
      new DocumentLoadInstrumentation(),
      new FetchInstrumentation({
        // The exporter's own posts would otherwise trace themselves.
        ignoreUrls: [/\/otlp\/v1\/traces$/],
        propagateTraceHeaderCorsUrls: [new RegExp(`^${window.location.origin.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}`)],
      }),
    ],
  })
  window.addEventListener('pagehide', () => { void provider.forceFlush() })
}
