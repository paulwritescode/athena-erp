# ADR-0001: Serve the UI and API from one Go server

- **Status:** Accepted
- **Date:** 2026-11-13
- **Decision owner:** Project owner
- **Related decisions:** D-002, D-003

## Context

The initial product should be simple to run and deploy. The agreed frontend is Vue 3 + TypeScript + Vite, while Go is the backend runtime and ConnectRPC is the RPC library. Production should not require a separate Node frontend server or an RPC proxy just to serve the application.

## Options considered

1. One Go process serves the API and embedded frontend assets.
2. A separately hosted frontend calls a separately hosted API.

## Decision

Build the Vue app with Vite, embed the resulting static files in the Go executable, and serve those files and ConnectRPC services from one Go HTTP server. `make run` is the normal repository-root entry point; `make build` creates a standalone executable containing the built UI.

## Consequences

- Simple self-hosted startup: one app process and one web origin.
- Node.js and npm are build-time requirements, not production runtime requirements.
- UI source changes require rebuilding the Vite output and Go binary.
- This decision does not remove the need for an external database in multi-user hosted deployments.
- The project can later add separate frontend hosting as an optional deployment profile if it solves a concrete need.

## Revisit when

Consider a separate frontend deployment only when there is a demonstrated deployment, caching, or scaling need that outweighs the simpler single-server workflow.
