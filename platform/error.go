package platform

import "fmt"

const (
	InternalError          = 20000
	MarshalError           = 20001
	BuildRequestError      = 20002
	SendRequestError       = 20003
	ReadResponseError      = 20004
	UnmarshalResponseError = 20005
	UnknownAPIVersion      = 20006
	InvalidInputError      = 20007
)

type RequestError struct {
	StatusCode int
	Err        error
}

func NewRequestError(statusCode int, err error) *RequestError {
	return &RequestError{
		StatusCode: statusCode,
		Err:        err,
	}
}

func (r *RequestError) Error() string {
	return fmt.Sprintf("status %d: %v", r.StatusCode, r.Err)
}
