package jsonrpc

//ParamsDataMarshaller interface for Request params
type ParamsDataMarshaller interface {
	MarshalJSON() ([]byte, error)
}
