package mockstruct

import "encoding/json"

//RequestTestStruct ...
type RequestTestStruct struct {
	ID int64
}

//MarshalJSON ...
func (v *RequestTestStruct) MarshalJSON() ([]byte, error) {
	return json.Marshal(*v)
}
