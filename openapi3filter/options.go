package openapi3filter

// DefaultOptions do not set an AuthenticationFunc.
// A spec with security schemes defined will not pass validation
// unless an AuthenticationFunc is defined.
var DefaultOptions = &Options{}

// Options used by ValidateRequest and ValidateResponse
type Options struct {
	// Set ExcludeRequestBody so ValidateRequest skips request body validation
	ExcludeRequestBody bool

	// Set ExcludeResponseBody so ValidateResponse skips response body validation
	ExcludeResponseBody bool

	// Set IncludeResponseStatus so ValidateResponse fails on response
	// status not defined in OpenAPI spec
	IncludeResponseStatus bool

	MultiError             bool
	FailOnUnknownParameter bool

	// See NoopAuthenticationFunc
	AuthenticationFunc AuthenticationFunc
}
