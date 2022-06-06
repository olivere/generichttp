package generichttp

import (
	"encoding/json"
	"io"
	"net/http"
)

// Handler for a generic endpoint.
type Handler[R, W any] func(http.ResponseWriter, Request[R]) (*Response[W], error)

// Request wraps data on the request side.
type Request[T any] struct {
	*http.Request
	Data *T
}

// NewRequest creates a new Request from a HTTP request.
func NewRequest[T any](r *http.Request) Request[T] {
	req := Request[T]{
		Request: r,
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req.Data)
	return req
}

// Response wraps data on the response side.
type Response[T any] struct {
	StatusCode int `json:"-"`
	Data       *T  `json:"data,omitempty"`
}

// NewResponse creates a new Response with the given data and HTTP status code
// 200. Use e.g. JSON to render as application/json.
func NewResponse[T any](data *T) *Response[T] {
	return NewResponseWithCode(http.StatusOK, data)
}

// NewResponseWithCode creates a new Response with the given HTTP status code.
// Use e.g. JSON to render as application/json.
func NewResponseWithCode[T any](code int, data *T) *Response[T] {
	resp := &Response[T]{
		StatusCode: code,
		Data:       data,
	}
	return resp
}

// JSON handles a request and returns a http.Handler. The request body is
// parsed and passed into the handler. The response returned from the handler
// is being encoded to JSON as well.
//
// If the handler returns an error, its is mapped as a JSON struct and
// a HTTP status code as well. Use e.g. BadRequestError to return specialized
// errors.
func JSON[R, W any](h Handler[R, W]) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := NewRequest[R](r)
		resp, err := h(w, req)
		if err != nil {
			WriteJSONError(w, err)
			return
		}
		if resp.Data != nil {
			WriteJSONCode(w, resp.StatusCode, resp.Data)
		}
	})
}

// WriteJSON renders JSON to the HTTP response body with HTTP status code 200.
func WriteJSON(w http.ResponseWriter, data any) {
	WriteJSONCode(w, http.StatusOK, data)
}

// WriteJSONCode renders JSON to the HTTP response body with the given
// HTTP status code.
func WriteJSONCode(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	if code == 0 {
		code = http.StatusOK
	}
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

// WriteJSONError renders the error as JSON. If the err has a HTTPCode() int
// function, it is being used for the HTTP status code. If the err has a
// HTTPError() string function, is it being used for the error message.
// Use specialized errors like BadRequestError to automatically do the right
// thing.
func WriteJSONError(w http.ResponseWriter, err error) {
	msg := struct {
		Message string `json:"message"`
	}{
		Message: "Internal server error",
	}
	w.Header().Set("Content-Type", "application/json")
	if intf, ok := err.(interface{ HTTPCode() int }); ok {
		w.WriteHeader(intf.HTTPCode())
	} else {
		w.WriteHeader(http.StatusInternalServerError)
	}
	if intf, ok := err.(interface{ HTTPError() string }); ok {
		msg.Message = intf.HTTPError()
	}
	_ = json.NewEncoder(w).Encode(msg)
}

// BadRequestError represents a HTTP Bad Request error (status code 400).
type BadRequestError struct {
	Message string
}

// Error implements the error interface.
func (e BadRequestError) Error() string { return e.HTTPError() }

// HTTPCode returns the HTTP code.
func (BadRequestError) HTTPCode() int { return http.StatusBadRequest }

// HTTPError returns the error message or "Bad request".
func (e BadRequestError) HTTPError() string {
	if e.Message != "" {
		return e.Message
	}
	return "Bad request"
}
