package badrequesterror

import (
	"fmt"
)

type BadRequestError struct {
	msg   string
	cause error
}

func New(format string, args ...interface{}) error {
	return &BadRequestError{msg: fmt.Sprintf(format, args...)}
}

func Wrap(err error, format string, args ...interface{}) error {
	return &BadRequestError{cause: err, msg: fmt.Sprintf(format, args...)}
}

func (e *BadRequestError) Error() string {
	if e.msg == "" {
		return "bad request"
	}
	return "bad request: " + e.msg
}

func (e *BadRequestError) Unwrap() error { return e.cause }
