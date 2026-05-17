package llamaserver

import "context"

// ServerInterface defines the contract for the llama-server sidecar.
// The concrete Server type satisfies this interface, allowing callers
// to mock the server lifecycle in tests.
type ServerInterface interface {
	Start(ctx context.Context) error
	Stop() error
	URL() string
}
