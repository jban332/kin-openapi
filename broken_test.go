package main

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/stretchr/testify/require"
)

func TestValidateRequest(t *testing.T) {
	spec := `openapi: 3.0.0
info:
  title: Example
  version: '1.0'
  contact:
    name: Chris Rodwell
    email: crodwell@github.com
  description: test
  termsOfService: test
servers:
  - url: 'http://localhost:3000'
    description: API
paths:
  /test:
    post:
      summary: Create
      tags:
        - test
      responses:
        '201':
          description: Created
          content:
            application/json:
              schema:
                type: object
                properties:
                  name:
                    type: string
                  parent_id:
                    type: integer
                  start_date:
                    type: string
                  end_date:
                    type: string
                  total_budget:
                    type: string
                  currency_code:
                    type: string
                  zone_name:
                    type: string
      operationId: post-test
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                name:
                  type: string
                  minLength: 1
                  maxLength: 256
                parent_id:
                  type: integer
                  format: int32
                start_date:
                  type: string
                  format: date-time
                end_date:
                  type: string
                  format: date-time
                total_budget:
                  type: number
                  format: float
                  minimum: 1
                  maximum: 99999999999999.98
                currency_code:
                  type: string
                  maxLength: 3
                zone_name:
                  type: string
              required:
                - name
                - parent_id
                - start_date
                - end_date
                - total_budget
                - currency_code
                - zone_name
            examples:
              Example:
                value:
                  parent_id: 190
                  name: valid test item
                  start_date: '2050-01-01T10:00:00Z'
                  end_date: '2050-01-10T23:59:00Z'
                  currency_code: USD
                  total_budget: 100
                  zone_name: America/New_York
        description: ''
      description: Create a test object
components:
  schemas: {}
tags:
  - name: test
    description: A test item

`

	loader := &openapi3.Loader{Context: context.Background()}
	doc, err := loader.LoadFromData([]byte(spec))
	require.NoError(t, err)
	err = doc.Validate(context.Background())
	require.NoError(t, err)
	router, err := legacy.NewRouter(doc)
	require.NoError(t, err)

	jsonBody := `{
		"parent_id": 190,
		"name": "valid test item",
		"start_date": "abc",
		"end_date": "2050-01-01T10:00:00Z",
		"currency_code": "USD",
		"total_budget": 100,
		"zone_name": "America/New_York"
	}`

	httpReq, err := http.NewRequest(http.MethodPost, "/test", strings.NewReader(jsonBody))
	require.NoError(t, err)

	// Find route
	route, pathParams, err := router.FindRoute(httpReq)
	require.NoError(t, err) // this fails with No Operation

	// Validate request
	requestValidationInput := &openapi3filter.RequestValidationInput{
		Request:    httpReq,
		PathParams: pathParams,
		Route:      route,
	}
	err = openapi3filter.ValidateRequest(context.Background(), requestValidationInput)
	require.NoError(t, err)

}
