package llmclient

import "errors"

var ErrInvalidJSON = errors.New("invalid json from LLM")

// PermanentError indicates an error that will not resolve with retries.
type PermanentError struct {
	Err error
}

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

func NewPermanentError(err error) error {
	return &PermanentError{Err: err}
}
