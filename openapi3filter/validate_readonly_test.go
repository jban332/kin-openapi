package openapi3filter

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	legacyrouter "github.com/getkin/kin-openapi/routers/legacy"
	"github.com/stretchr/testify/require"
)

func TestValidatingRequestBodyWithReadOnlyProperty(t *testing.T) {
	const spec = `{
  "openapi": "3.0.3",
  "info": {
    "version": "1.0.0",
    "title": "title",
    "description": "desc",
    "contact": {
      "email": "email"
    }
  },
  "paths": {
    "/accounts": {
      "post": {
        "description": "Create a new account",
        "parameters": [
          {
            "in": "query",
            "name": "q",
            "schema": {
              "type": "string",
              "default": "Q"
            }
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["_id"],
                "properties": {
                  "_": {
                    "type": "boolean",
                    "default": false
                  },
                  "_id": {
                    "type": "string",
                    "description": "Unique identifier for this object.",
                    "pattern": "[0-9a-v]+$",
                    "minLength": 20,
                    "maxLength": 20,
                    "readOnly": true
                  }
                }
              }
            }
          }
        },
        "responses": {
          "201": {
            "description": "Successfully created a new account"
          },
          "400": {
            "description": "The server could not understand the request due to invalid syntax",
          }
        }
      }
    }
  }
}
`

	sl := openapi3.NewSwaggerLoader()
	doc, err := sl.LoadSwaggerFromData([]byte(spec))
	require.NoError(t, err)
	err = doc.Validate(sl.Context)
	require.NoError(t, err)
	router, err := legacyrouter.NewRouter(doc)
	require.NoError(t, err)

	b, err := json.Marshal(struct {
		Blank bool   `json:"_,omitempty"`
		ID    string `json:"_id"`
	}{
		ID: "bt6kdc3d0cvp6u8u3ft0",
	})
	require.NoError(t, err)

	httpReq, err := http.NewRequest(http.MethodPost, "/accounts", bytes.NewReader(b))
	require.NoError(t, err)
	httpReq.Header.Add(headerCT, "application/json")

	route, pathParams, err := router.FindRoute(httpReq)
	require.NoError(t, err)

	err = ValidateRequest(sl.Context, &RequestValidationInput{
		Request:    httpReq,
		PathParams: pathParams,
		Route:      route,
	})
	require.NoError(t, err)

	// Unset default values in body were set
	validatedReqBody, err := ioutil.ReadAll(httpReq.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"_":false,"_id":"bt6kdc3d0cvp6u8u3ft0"}`, string(validatedReqBody))
	// Unset default values in URL were set
	// Unset default values in headers were set
	// Unset default values in cookies were set
}
