# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Engineering Policies

These are _strict_ policies that must be followed by all engineers and developers in this project. PRs will be rejected if these policies are violated.

### Dependency Management

- All dependencies _must_ be added, removed and updated using `go get` on the command line.
- Under no circumstances should `go.mod` be manually edited with regard to dependencies.

### Coding

#### Testing

- **SHOULD** test-driven development

#### Dependencies

- **SHOULD** prefer standard library; introduce dependencies only with clear value

#### Code Style

- **MUST** enforce `gofmt`
- **MUST** avoid stutter in names: `package kv; type Store` (not `KVStore` in `kv`)
- **MUST** channels for orchestration, mutexes for state
- **MUST** dependency injection via interfaces
- **SHOULD** prefer composition over inheritance
- **SHOULD** small interfaces near consumers
- **SHOULD** prefer generics when it clarifies code
- **SHOULD** accept interfaces, return structs

#### Errors

- Wrapped errors with context
- Custom error types with behavior
- Sentinel errors for known conditions
- Error handling at appropriate levels
- Structured error messages
- Error recovery strategies
- Panic only for programming errors
- Graceful degradation patterns

#### Concurrency

- Goroutine lifecycle management
- Channel patterns and pipelines
- Context for cancellation and deadlines
- Select statements for multiplexing
- Worker pools with bounded concurrency
- Fan-in/fan-out patterns
- Rate limiting and backpressure
- Synchronization with sync primitives
