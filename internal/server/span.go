package server

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// startKernelSpan opens a span for one kernel write and returns the request
// ID of the call (a new one when the call has none yet). The span carries only the operation, the Record Type key, the
// request ID and the outcome code, never a Record value. With no OTLP
// endpoint the global tracer is a no-op.
func startKernelSpan(ctx context.Context, operation, recordTypeKey string) (context.Context, string, func(err error)) {
	ctx, requestID := withRequest(ctx)
	ctx, span := otel.Tracer("athena/kernel").Start(ctx, "athena."+operation, trace.WithAttributes(
		attribute.String("athena.operation", operation),
		attribute.String("athena.record_type", recordTypeKey),
		attribute.String("athena.request_id", requestID),
	))
	return ctx, requestID, func(err error) {
		outcome := "ok"
		if err != nil {
			outcome = connect.CodeOf(err).String()
			span.SetStatus(codes.Error, outcome)
		}
		span.SetAttributes(attribute.String("athena.outcome", outcome))
		span.End()
	}
}

// newRequestID returns the correlation ID stored in the request_id column.
func newRequestID() string {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.NewString()
	}
	return id.String()
}

func connectMessage(err error) string {
	var ce *connect.Error
	if errors.As(err, &ce) {
		return ce.Message()
	}
	return fmt.Sprint(err)
}
