package jsonrpc

// ParamsDataMarshaler is the interface for request params that can be marshaled to JSON.
type ParamsDataMarshaler interface {
	MarshalJSON() ([]byte, error)
}
