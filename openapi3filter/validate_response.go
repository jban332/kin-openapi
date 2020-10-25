// Package openapi3filter validates that requests and inputs request an OpenAPI 3 specification file.
package openapi3filter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
)

// ValidateResponse is used to validate the given input according to previous
// loaded OpenAPIv3 spec. If the input does not match the OpenAPIv3 spec, a
// non-nil error will be returned.
//
// Note: One can tune the behavior of uniqueItems: true verification
// by registering a custom function with openapi3.RegisterArrayUniqueItemsChecker
func ValidateResponse(c context.Context, input *ResponseValidationInput) error {
	req := input.RequestValidationInput.Request
	switch req.Method {
	case "HEAD":
		return nil
	}
	status := input.Status
	if status < 100 {
		return &ResponseError{
			Input:  input,
			Reason: "illegal status code",
			Err:    fmt.Errorf("Status %d", status),
		}
	}

	// These status codes will never be validated.
	// TODO: The list is probably missing some.
	switch status {
	case http.StatusNotModified,
		http.StatusPermanentRedirect,
		http.StatusTemporaryRedirect,
		http.StatusMovedPermanently:
		return nil
	}
	route := input.RequestValidationInput.Route
	options := input.Options
	if options == nil {
		options = DefaultOptions
	}

	// Find input for the current status
	responses := route.Operation.Responses
	if len(responses) == 0 {
		return nil
	}
	responseRef := responses.Get(status) // Response
	if responseRef == nil {
		responseRef = responses.Default() // Default input
	}
	if responseRef == nil {
		// By default, status that is not documented is allowed.
		if !options.IncludeResponseStatus {
			return nil
		}

		return &ResponseError{Input: input, Reason: "status is not supported"}
	}
	response := responseRef.Value
	if response == nil {
		return &ResponseError{Input: input, Reason: "response has not been resolved"}
	}

	if options.ExcludeResponseBody {
		// A user turned off validation of a response's body.
		return nil
	}

	content := response.Content
	if len(content) == 0 || options.ExcludeResponseBody {
		// An operation does not contains a validation schema for responses with this status code.
		return nil
	}

	inputMIME := input.Header.Get("Content-Type")
	contentType := content.Get(inputMIME)
	if contentType == nil {
		return &ResponseError{
			Input:  input,
			Reason: fmt.Sprintf("input header 'Content-Type' has unexpected value: %q", inputMIME),
		}
	}

	if contentType.Schema == nil {
		// An operation does not contains a validation schema for responses with this status code.
		return nil
	}

	// Read response's body.
	body := input.Body

	// Response would contain partial or empty input body
	// after we begin reading.
	// Ensure that this doesn't happen.
	input.Body = nil

	// Ensure we close the reader
	defer body.Close()

	// Read all
	data, err := ioutil.ReadAll(body)
	if err != nil {
		return &ResponseError{
			Input:  input,
			Reason: "failed to read response body",
			Err:    err,
		}
	}

	// Put the data back into the response.
	input.SetBodyBytes(data)

	encFn := func(name string) *openapi3.Encoding { return contentType.Encoding[name] }
	value, err := decodeBody(bytes.NewBuffer(data), input.Header, contentType.Schema, encFn)
	if err != nil {
		return &ResponseError{
			Input:  input,
			Reason: "failed to decode response body",
			Err:    err,
		}
	}

	if input.Options.TrimAdditionalProperties {
		// if additionalProperties is false ,filter body
		//if !contentType.Schema.additionalProperties
		value = TrimAdditionalProperties(value, contentType.Schema.Value)
		newData, err := json.Marshal(value)
		if err != nil {
			return &ResponseError{
				Input:  input,
				Reason: "reset response body error",
				Err:    err,
			}
		}
		input.SetBodyBytes(newData)
	}
	// Validate data with the schema.
	if err := contentType.Schema.Value.VisitJSON(value, openapi3.VisitAsResponse()); err != nil {
		return &ResponseError{
			Input:  input,
			Reason: "response body doesn't match the schema",
			Err:    err,
		}
	}
	return nil
}

//TrimAdditionalProperties remove not allowd properties
func TrimAdditionalProperties(value interface{}, schema *openapi3.Schema) (output interface{}) {
	switch value := value.(type) {
	case []interface{}:
		return trimArray(value, schema)
	case map[string]interface{}:
		return trimObject(value, schema)
	default:
		output = value
	}
	return

}

func trimArray(value []interface{}, schema *openapi3.Schema) (output []interface{}) {
	itemSchemaRef := schema.Items
	if itemSchemaRef == nil {
		output = value // if empty , return original value 
		return
	}

	itemSchema := itemSchemaRef.Value
	if itemSchema == nil { // return original value if schema nil
		output = value
		return
	}

	output = make([]interface{}, len(value))
	for i, item := range value {
		item = TrimAdditionalProperties(item, itemSchema)
		output[i] = item
	}
	return
}

func trimObject(value map[string]interface{}, schema *openapi3.Schema) (output map[string]interface{}) {
	output = make(map[string]interface{}, 0)
	allowed := schema.AdditionalPropertiesAllowed
	notAllowed := allowed != nil && !*allowed
	properties := schema.Properties
	if properties == nil {

		if notAllowed {
			return
		}
		output = value
		return
	}
	for k, v := range value {
		propertyRef, ok := properties[k]
		if !ok { 
			if !notAllowed { // add schema allow properties
				output[k] = v
			}
			continue

		}

		if propertyRef.Value != nil {
			output[k] = TrimAdditionalProperties(v, propertyRef.Value)
		} else {
			output[k] = v
		}

	}
	return
}
