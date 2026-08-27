package domain

import "fmt"

type ErrorCode string

const (
	CodeValidation ErrorCode = "validation"
	CodeNotFound   ErrorCode = "not_found"
	CodeConflict   ErrorCode = "conflict"
	CodeFrozen     ErrorCode = "frozen"
	CodeState      ErrorCode = "invalid_state"
)

type BusinessError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Details any       `json:"details,omitempty"`
}

func NewDetailedError(code ErrorCode, message string, details any) error {
	return &BusinessError{Code: code, Message: message, Details: details}
}

func ErrorDetails(err error) any {
	if business, ok := err.(*BusinessError); ok {
		return business.Details
	}
	return nil
}

func (e *BusinessError) Error() string { return e.Message }

func NewError(code ErrorCode, format string, args ...any) error {
	return &BusinessError{Code: code, Message: fmt.Sprintf(format, args...)}
}

func ErrorCodeOf(err error) ErrorCode {
	if business, ok := err.(*BusinessError); ok {
		return business.Code
	}
	return "internal"
}
