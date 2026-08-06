package errs

import "fmt"

type Kind string

const (
	KindUsage         Kind = "usage_error"
	KindInput         Kind = "input_error"
	KindValidation    Kind = "validation_error"
	KindConfiguration Kind = "configuration_error"
	KindTransport     Kind = "transport_error"
	KindAPI           Kind = "api_error"
	KindInterrupted   Kind = "interrupted"
)

type Error struct {
	Kind       Kind
	Message    string
	Details    any
	StatusCode int
	RequestID  string
}

func (e *Error) Error() string { return e.Message }

func New(kind Kind, message string) *Error {
	return &Error{Kind: kind, Message: message}
}

func Wrap(kind Kind, message string, err error) *Error {
	return &Error{Kind: kind, Message: fmt.Sprintf("%s: %v", message, err)}
}

func ExitCode(kind Kind) int {
	switch kind {
	case KindConfiguration, KindTransport:
		return 3
	case KindAPI:
		return 4
	case KindInterrupted:
		return 130
	default:
		return 2
	}
}
