package server

import (
	"context"

	"connectrpc.com/connect/v2"
	"github.com/google/uuid"
)

// requestKey carries the correlation ID and source of one request.
type requestKey struct{}

type requestInfo struct {
	id, source string
}

// Sources written to the Audit Log (spec 12.1).
const (
	sourceAPI = "api"
	sourceCLI = "cli"
)

// requestIDHeader carries the correlation ID in both directions.
const requestIDHeader = "X-Request-Id"

// requestInterceptor gives every ConnectRPC call one request ID. A client
// may send its own as a UUID in X-Request-Id; anything else is replaced. The
// ID goes back in the response header and is stored on the rows, Audit Log
// entries, versions and activity the call writes (spec 12.2).
func requestInterceptor() connect.ServerInterceptor {
	return func(next connect.ServerFunc) connect.ServerFunc {
		return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
			id := ""
			if info, ok := connect.CallInfoForServerContext(ctx); ok {
				if sent := info.RequestHeader().Get(requestIDHeader); sent != "" {
					if parsed, err := uuid.Parse(sent); err == nil {
						id = parsed.String()
					}
				}
				if id == "" {
					id = newRequestID()
				}
				info.ResponseHeader().Set(requestIDHeader, id)
			} else {
				id = newRequestID()
			}
			return next(context.WithValue(ctx, requestKey{}, requestInfo{id: id, source: sourceAPI}), spec, stream)
		}
	}
}

// requestFrom returns the request ID and source of ctx. Calls that did not
// come through the interceptor (the CLI) get a new ID and the cli source.
func requestFrom(ctx context.Context) (id, source string) {
	if info, ok := ctx.Value(requestKey{}).(requestInfo); ok {
		return info.id, info.source
	}
	return newRequestID(), sourceCLI
}

// withRequest stores the request ID in ctx unless it already holds one.
func withRequest(ctx context.Context) (context.Context, string) {
	if info, ok := ctx.Value(requestKey{}).(requestInfo); ok {
		return ctx, info.id
	}
	id, source := requestFrom(ctx)
	return context.WithValue(ctx, requestKey{}, requestInfo{id: id, source: source}), id
}
