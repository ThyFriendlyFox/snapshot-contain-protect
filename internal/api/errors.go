package api

import "net/http"

// apiError carries the status code the handler sends. Every caller-visible
// failure states what is wrong in one sentence.
type apiError struct {
	status  int
	message string
}

func (e apiError) Error() string { return e.message }

func badRequest(msg string) error  { return apiError{http.StatusBadRequest, msg} }
func notFound(msg string) error    { return apiError{http.StatusNotFound, msg} }
func conflict(msg string) error    { return apiError{http.StatusConflict, msg} }
func unavailable(msg string) error { return apiError{http.StatusNotImplemented, msg} }
