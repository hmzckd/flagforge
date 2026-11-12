# One evaluation package for the server and the SDK

The rules that decide which value a flag serves live in one public, pure package, `eval`, at the root of the module.
The server's evaluate endpoint and the Go SDK both call it. We considered keeping it under `internal/` and writing the
same logic again inside the SDK, but `internal/` packages cannot be imported by other modules, and two copies of the
rules would drift apart until the server and the SDK gave different answers for the same user. The cost is that `eval`
is now part of the module's public API, so changing its exported types is a breaking change for SDK users.
