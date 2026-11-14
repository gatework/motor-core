// Package encoder defines serialization for configuration sources.
package encoder

// Encoder converts configuration values to and from a named format.
//
//nolint:iface // Shared serialization contract is injected by both reader and source packages.
type Encoder interface {
	Encode(value any) ([]byte, error)
	Decode(data []byte, value any) error
	String() string
}
