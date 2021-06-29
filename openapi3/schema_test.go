package openapi3

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type schemaExample struct {
	Title         string
	Schema        *Schema
	Serialization interface{}
	AllValid      []interface{}
	AllInvalid    []interface{}
}

func TestSchemas(t *testing.T) {
	DefineStringFormat("uuid", FormatOfStringForUUIDOfRFC4122)
	for _, example := range schemaExamples {
		t.Run(example.Title, testSchema(t, example))
	}
}

func testSchema(t *testing.T, example schemaExample) func(*testing.T) {
	return func(t *testing.T) {
		schema := example.Schema
		if serialized := example.Serialization; serialized != nil {
			jsonSerialized, err := json.Marshal(serialized)
			require.NoError(t, err)
			jsonSchema, err := json.Marshal(schema)
			require.NoError(t, err)
			require.JSONEq(t, string(jsonSerialized), string(jsonSchema))
			var dataUnserialized Schema
			err = json.Unmarshal(jsonSerialized, &dataUnserialized)
			require.NoError(t, err)
			var dataSchema Schema
			err = json.Unmarshal(jsonSchema, &dataSchema)
			require.NoError(t, err)
			require.Equal(t, dataUnserialized, dataSchema)
		}
		for _, value := range example.AllValid {
			err := validateSchema(t, schema, value)
			require.NoError(t, err)
		}
		for _, value := range example.AllInvalid {
			err := validateSchema(t, schema, value)
			require.Error(t, err)
		}
		// NaN and Inf aren't valid JSON but are handled
		for _, value := range []interface{}{math.NaN(), math.Inf(-1), math.Inf(+1)} {
			err := schema.VisitJSON(value)
			require.Error(t, err)
		}
	}
}

func validateSchema(t *testing.T, schema *Schema, value interface{}, opts ...SchemaValidationOption) error {
	data, err := json.Marshal(value)
	require.NoError(t, err)
	var val interface{}
	err = json.Unmarshal(data, &val)
	require.NoError(t, err)
	return schema.VisitJSON(val, opts...)
}

var schemaExamples = []schemaExample{
	{
		Title:         "EMPTY SCHEMA",
		Schema:        &Schema{},
		Serialization: map[string]interface{}{
			// This OA3 schema is exactly this draft-04 schema:
			//   {"not": {"type": "null"}}
		},
		AllValid: []interface{}{
			false,
			true,
			3.14,
			"",
			[]interface{}{},
			map[string]interface{}{},
		},
		AllInvalid: []interface{}{
			nil,
		},
	},

	{
		Title:  "JUST NULLABLE",
		Schema: NewSchema().WithNullable(),
		Serialization: map[string]interface{}{
			// This OA3 schema is exactly both this draft-04 schema: {} and:
			// {anyOf: [type:string, type:number, type:integer, type:boolean
			//         ,{type:array, items:{}}, type:object]}
			"nullable": true,
		},
		AllValid: []interface{}{
			nil,
			false,
			true,
			0,
			0.0,
			3.14,
			"",
			[]interface{}{},
			map[string]interface{}{},
		},
	},

	{
		Title:  "NULLABLE BOOLEAN",
		Schema: NewBoolSchema().WithNullable(),
		Serialization: map[string]interface{}{
			"nullable": true,
			"type":     "boolean",
		},
		AllValid: []interface{}{
			nil,
			false,
			true,
		},
		AllInvalid: []interface{}{
			0,
			0.0,
			3.14,
			"",
			[]interface{}{},
			map[string]interface{}{},
		},
	},

	{
		Title: "NULLABLE ANYOF",
		Schema: NewAnyOfSchema(
			NewIntegerSchema(),
			NewFloat64Schema(),
		).WithNullable(),
		Serialization: map[string]interface{}{
			"nullable": true,
			"anyOf": []interface{}{
				map[string]interface{}{"type": "integer"},
				map[string]interface{}{"type": "number"},
			},
		},
		AllValid: []interface{}{
			nil,
			42,
			4.2,
		},
		AllInvalid: []interface{}{
			true,
			[]interface{}{42},
			"bla",
			map[string]interface{}{},
		},
	},

	{
		Title: "NULLABLE ALLOF",
		Schema: &Schema{
			Type: "object",
			Properties: map[string]*SchemaRef{
				"value": NewAllOfSchema(
					NewBoolSchema().WithNullable(),
					NewBoolSchema().WithNullable(),
				).NewRef()}},
		Serialization: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"value": map[string]interface{}{
					"allOf": []interface{}{
						map[string]interface{}{
							"nullable": true,
							"type":     "boolean",
						},
						map[string]interface{}{
							"nullable": true,
							"type":     "boolean",
						},
					}}}},
		AllValid: []interface{}{
			nil,
			true,
			false,
		},
		AllInvalid: []interface{}{
			2,
			4.2,
			[]interface{}{42},
			"bla",
			map[string]interface{}{},
		},
	},

	{
		Title:  "BOOLEAN",
		Schema: NewBoolSchema(),
		Serialization: map[string]interface{}{
			"type": "boolean",
		},
		AllValid: []interface{}{
			false,
			true,
		},
		AllInvalid: []interface{}{
			nil,
			3.14,
			"",
			[]interface{}{},
			map[string]interface{}{},
		},
	},

	{
		Title: "NUMBER",
		Schema: NewFloat64Schema().
			WithMin(2.5).
			WithMax(3.5),
		Serialization: map[string]interface{}{
			"type":    "number",
			"minimum": 2.5,
			"maximum": 3.5,
		},
		AllValid: []interface{}{
			2.5,
			3.14,
			3.5,
		},
		AllInvalid: []interface{}{
			nil,
			false,
			true,
			2.4,
			3.6,
			"",
			[]interface{}{},
			map[string]interface{}{},
		},
	},

	{
		Title: "INTEGER",
		Schema: NewInt64Schema().
			WithMin(2).
			WithMax(5),
		Serialization: map[string]interface{}{
			"type":    "integer",
			"format":  "int64",
			"minimum": 2,
			"maximum": 5,
		},
		AllValid: []interface{}{
			2,
			5,
		},
		AllInvalid: []interface{}{
			nil,
			false,
			true,
			1,
			6,
			3.5,
			"",
			[]interface{}{},
			map[string]interface{}{},
		},
	},

	{
		Title: "STRING",
		Schema: NewStringSchema().
			WithMinLength(2).
			WithMaxLength(3).
			WithPattern("^[abc]+$"),
		Serialization: map[string]interface{}{
			"type":      "string",
			"minLength": 2,
			"maxLength": 3,
			"pattern":   "^[abc]+$",
		},
		AllValid: []interface{}{
			"ab",
			"abc",
		},
		AllInvalid: []interface{}{
			nil,
			false,
			true,
			3.14,
			"a",
			"xy",
			"aaaa",
			[]interface{}{},
			map[string]interface{}{},
		},
	},

	{
		Title:  "STRING: optional format 'uuid'",
		Schema: NewUUIDSchema(),
		Serialization: map[string]interface{}{
			"type":   "string",
			"format": "uuid",
		},
		AllValid: []interface{}{
			"dd7d8481-81a3-407f-95f0-a2f1cb382a4b",
			"dcba3901-2fba-48c1-9db2-00422055804e",
			"ace8e3be-c254-4c10-8859-1401d9a9d52a",
		},
		AllInvalid: []interface{}{
			nil,
			"g39840b1-d0ef-446d-e555-48fcca50a90a",
			"4cf3i040-ea14-4daa-b0b5-ea9329473519",
			"aaf85740-7e27-4b4f-b4554-a03a43b1f5e3",
			"56f5bff4-z4b6-48e6-a10d-b6cf66a83b04",
		},
	},

	{
		Title:  "STRING: format 'date-time'",
		Schema: NewDateTimeSchema(),
		Serialization: map[string]interface{}{
			"type":   "string",
			"format": "date-time",
		},
		AllValid: []interface{}{
			"2017-12-31T11:59:59",
			"2017-12-31T11:59:59Z",
			"2017-12-31T11:59:59-11:30",
			"2017-12-31T11:59:59+11:30",
			"2017-12-31T11:59:59.999+11:30",
			"2017-12-31T11:59:59.999Z",
		},
		AllInvalid: []interface{}{
			nil,
			3.14,
			"2017-12-31",
			"2017-12-31T11:59:59\n",
			"2017-12-31T11:59:59.+11:30",
			"2017-12-31T11:59:59.Z",
		},
	},

	{
		Title:  "STRING: format 'date-time'",
		Schema: NewBytesSchema(),
		Serialization: map[string]interface{}{
			"type":   "string",
			"format": "byte",
		},
		AllValid: []interface{}{
			"",
			base64.StdEncoding.EncodeToString(func() []byte {
				data := make([]byte, 0, 1024)
				for i := 0; i < cap(data); i++ {
					data = append(data, byte(i))
				}
				return data
			}()),
			base64.URLEncoding.EncodeToString(func() []byte {
				data := make([]byte, 0, 1024)
				for i := 0; i < cap(data); i++ {
					data = append(data, byte(i))
				}
				return data
			}()),
		},
		AllInvalid: []interface{}{
			nil,
			" ",
			"\n",
			"%",
		},
	},

	{
		Title: "ARRAY",
		Schema: &Schema{
			Type:        "array",
			MinItems:    2,
			MaxItems:    Uint64Ptr(3),
			UniqueItems: true,
			Items:       NewFloat64Schema().NewRef(),
		},
		Serialization: map[string]interface{}{
			"type":        "array",
			"minItems":    2,
			"maxItems":    3,
			"uniqueItems": true,
			"items": map[string]interface{}{
				"type": "number",
			},
		},
		AllValid: []interface{}{
			[]interface{}{
				1, 2,
			},
			[]interface{}{
				1, 2, 3,
			},
		},
		AllInvalid: []interface{}{
			nil,
			3.14,
			[]interface{}{
				1,
			},
			[]interface{}{
				42, 42,
			},
			[]interface{}{
				1, 2, 3, 4,
			},
		},
	},
	{
		Title: "ARRAY : items format 'object'",
		Schema: &Schema{
			Type:        "array",
			UniqueItems: true,
			Items: (&Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"key1": NewFloat64Schema().NewRef(),
				},
			}).NewRef(),
		},
		Serialization: map[string]interface{}{
			"type":        "array",
			"uniqueItems": true,
			"items": map[string]interface{}{
				"properties": map[string]interface{}{
					"key1": map[string]interface{}{
						"type": "number",
					},
				},
				"type": "object",
			},
		},
		AllValid: []interface{}{
			[]interface{}{
				map[string]interface{}{
					"key1": 1,
					"key2": 1,
					// Additioanl properties will make object different
					// By default additionalProperties is true
				},
				map[string]interface{}{
					"key1": 1,
				},
			},
			[]interface{}{
				map[string]interface{}{
					"key1": 1,
				},
				map[string]interface{}{
					"key1": 2,
				},
			},
		},
		AllInvalid: []interface{}{
			[]interface{}{
				map[string]interface{}{
					"key1": 1,
				},
				map[string]interface{}{
					"key1": 1,
				},
			},
		},
	},

	{
		Title: "ARRAY : items format 'object' and object with a property of array type ",
		Schema: &Schema{
			Type:        "array",
			UniqueItems: true,
			Items: (&Schema{
				Type: "object",
				Properties: map[string]*SchemaRef{
					"key1": (&Schema{
						Type:        "array",
						UniqueItems: true,
						Items:       NewFloat64Schema().NewRef(),
					}).NewRef(),
				},
			}).NewRef(),
		},
		Serialization: map[string]interface{}{
			"type":        "array",
			"uniqueItems": true,
			"items": map[string]interface{}{
				"properties": map[string]interface{}{
					"key1": map[string]interface{}{
						"type":        "array",
						"uniqueItems": true,
						"items": map[string]interface{}{
							"type": "number",
						},
					},
				},
				"type": "object",
			},
		},
		AllValid: []interface{}{
			[]interface{}{
				map[string]interface{}{
					"key1": []interface{}{
						1, 2,
					},
				},
				map[string]interface{}{
					"key1": []interface{}{
						3, 4,
					},
				},
			},
			[]interface{}{ // Slice have items with the same value but with different index will treated as different slices
				map[string]interface{}{
					"key1": []interface{}{
						10, 9,
					},
				},
				map[string]interface{}{
					"key1": []interface{}{
						9, 10,
					},
				},
			},
		},
		AllInvalid: []interface{}{
			[]interface{}{ // Violate outer array uniqueItems: true
				map[string]interface{}{
					"key1": []interface{}{
						9, 9,
					},
				},
				map[string]interface{}{
					"key1": []interface{}{
						9, 9,
					},
				},
			},
			[]interface{}{ // Violate inner(array in object) array uniqueItems: true
				map[string]interface{}{
					"key1": []interface{}{
						9, 9,
					},
				},
				map[string]interface{}{
					"key1": []interface{}{
						8, 8,
					},
				},
			},
		},
	},

	{
		Title: "ARRAY : items format 'array'",
		Schema: &Schema{
			Type:        "array",
			UniqueItems: true,
			Items: (&Schema{
				Type:        "array",
				UniqueItems: true,
				Items:       NewFloat64Schema().NewRef(),
			}).NewRef(),
		},
		Serialization: map[string]interface{}{
			"type":        "array",
			"uniqueItems": true,
			"items": map[string]interface{}{
				"items": map[string]interface{}{
					"type": "number",
				},
				"uniqueItems": true,
				"type":        "array",
			},
		},
		AllValid: []interface{}{
			[]interface{}{
				[]interface{}{1, 2},
				[]interface{}{3, 4},
			},
			[]interface{}{ // Slice have items with the same value but with different index will treated as different slices
				[]interface{}{1, 2},
				[]interface{}{2, 1},
			},
		},
		AllInvalid: []interface{}{
			[]interface{}{ // Violate outer array uniqueItems: true
				[]interface{}{8, 9},
				[]interface{}{8, 9},
			},
			[]interface{}{ // Violate inner array uniqueItems: true
				[]interface{}{9, 9},
				[]interface{}{8, 8},
			},
		},
	},

	{
		Title: "ARRAY : items format 'array' and array with object type items",
		Schema: &Schema{
			Type:        "array",
			UniqueItems: true,
			Items: (&Schema{
				Type:        "array",
				UniqueItems: true,
				Items: (&Schema{
					Type: "object",
					Properties: map[string]*SchemaRef{
						"key1": NewFloat64Schema().NewRef(),
					},
				}).NewRef(),
			}).NewRef(),
		},
		Serialization: map[string]interface{}{
			"type":        "array",
			"uniqueItems": true,
			"items": map[string]interface{}{
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"key1": map[string]interface{}{
							"type": "number",
						},
					},
				},
				"uniqueItems": true,
				"type":        "array",
			},
		},
		AllValid: []interface{}{
			[]interface{}{
				[]interface{}{
					map[string]interface{}{
						"key1": 1,
					},
				},
				[]interface{}{
					map[string]interface{}{
						"key1": 2,
					},
				},
			},
			[]interface{}{ // Slice have items with the same value but with different index will treated as different slices
				[]interface{}{
					map[string]interface{}{
						"key1": 1,
					},
					map[string]interface{}{
						"key1": 2,
					},
				},
				[]interface{}{
					map[string]interface{}{
						"key1": 2,
					},
					map[string]interface{}{
						"key1": 1,
					},
				},
			},
		},
		AllInvalid: []interface{}{
			[]interface{}{ // Violate outer array uniqueItems: true
				[]interface{}{
					map[string]interface{}{
						"key1": 1,
					},
					map[string]interface{}{
						"key1": 2,
					},
				},
				[]interface{}{
					map[string]interface{}{
						"key1": 1,
					},
					map[string]interface{}{
						"key1": 2,
					},
				},
			},
			[]interface{}{ // Violate inner array uniqueItems: true
				[]interface{}{
					map[string]interface{}{
						"key1": 1,
					},
					map[string]interface{}{
						"key1": 1,
					},
				},
				[]interface{}{
					map[string]interface{}{
						"key1": 2,
					},
					map[string]interface{}{
						"key1": 2,
					},
				},
			},
		},
	},

	{
		Title: "OBJECT",
		Schema: &Schema{
			Type:     "object",
			MaxProps: Uint64Ptr(2),
			Properties: map[string]*SchemaRef{
				"numberProperty": NewFloat64Schema().NewRef(),
			},
		},
		Serialization: map[string]interface{}{
			"type":          "object",
			"maxProperties": 2,
			"properties": map[string]interface{}{
				"numberProperty": map[string]interface{}{
					"type": "number",
				},
			},
		},
		AllValid: []interface{}{
			map[string]interface{}{},
			map[string]interface{}{
				"numberProperty": 3.14,
			},
			map[string]interface{}{
				"numberProperty": 3.14,
				"some prop":      nil,
			},
		},
		AllInvalid: []interface{}{
			nil,
			false,
			true,
			3.14,
			"",
			[]interface{}{},
			map[string]interface{}{
				"numberProperty": "abc",
			},
			map[string]interface{}{
				"numberProperty": 3.14,
				"some prop":      42,
				"third":          "prop",
			},
		},
	},
	{
		Schema: &Schema{
			Type: "object",
			AdditionalProperties: &SchemaRef{
				Value: &Schema{
					Type: "number",
				},
			},
		},
		Serialization: map[string]interface{}{
			"type": "object",
			"additionalProperties": map[string]interface{}{
				"type": "number",
			},
		},
		AllValid: []interface{}{
			map[string]interface{}{},
			map[string]interface{}{
				"x": 3.14,
				"y": 3.14,
			},
		},
		AllInvalid: []interface{}{
			map[string]interface{}{
				"x": "abc",
			},
		},
	},
	{
		Schema: &Schema{
			Type:                        "object",
			AdditionalPropertiesAllowed: BoolPtr(true),
		},
		Serialization: map[string]interface{}{
			"type":                 "object",
			"additionalProperties": true,
		},
		AllValid: []interface{}{
			map[string]interface{}{},
			map[string]interface{}{
				"x": false,
				"y": 3.14,
			},
		},
	},

	{
		Title: "NOT",
		Schema: &Schema{
			Not: &SchemaRef{
				Value: &Schema{
					Enum: []interface{}{
						nil,
						true,
						3.14,
						"not this",
					},
				},
			},
		},
		Serialization: map[string]interface{}{
			"not": map[string]interface{}{
				"enum": []interface{}{
					nil,
					true,
					3.14,
					"not this",
				},
			},
		},
		AllValid: []interface{}{
			false,
			2,
			"abc",
		},
		AllInvalid: []interface{}{
			nil,
			true,
			3.14,
			"not this",
		},
	},

	{
		Title: "ANY OF",
		Schema: &Schema{
			AnyOf: []*SchemaRef{
				{
					Value: NewFloat64Schema().
						WithMin(1).
						WithMax(2),
				},
				{
					Value: NewFloat64Schema().
						WithMin(2).
						WithMax(3),
				},
			},
		},
		Serialization: map[string]interface{}{
			"anyOf": []interface{}{
				map[string]interface{}{
					"type":    "number",
					"minimum": 1,
					"maximum": 2,
				},
				map[string]interface{}{
					"type":    "number",
					"minimum": 2,
					"maximum": 3,
				},
			},
		},
		AllValid: []interface{}{
			1,
			2,
			3,
		},
		AllInvalid: []interface{}{
			0,
			4,
		},
	},

	{
		Title: "ALL OF",
		Schema: &Schema{
			AllOf: []*SchemaRef{
				{
					Value: NewFloat64Schema().
						WithMin(1).
						WithMax(2),
				},
				{
					Value: NewFloat64Schema().
						WithMin(2).
						WithMax(3),
				},
			},
		},
		Serialization: map[string]interface{}{
			"allOf": []interface{}{
				map[string]interface{}{
					"type":    "number",
					"minimum": 1,
					"maximum": 2,
				},
				map[string]interface{}{
					"type":    "number",
					"minimum": 2,
					"maximum": 3,
				},
			},
		},
		AllValid: []interface{}{
			2,
		},
		AllInvalid: []interface{}{
			0,
			1,
			3,
			4,
		},
	},

	{
		Title: "ONE OF",
		Schema: &Schema{
			OneOf: []*SchemaRef{
				{
					Value: NewFloat64Schema().
						WithMin(1).
						WithMax(2),
				},
				{
					Value: NewFloat64Schema().
						WithMin(2).
						WithMax(3),
				},
			},
		},
		Serialization: map[string]interface{}{
			"oneOf": []interface{}{
				map[string]interface{}{
					"type":    "number",
					"minimum": 1,
					"maximum": 2,
				},
				map[string]interface{}{
					"type":    "number",
					"minimum": 2,
					"maximum": 3,
				},
			},
		},
		AllValid: []interface{}{
			1,
			3,
		},
		AllInvalid: []interface{}{
			0,
			2,
			4,
		},
	},
}

type schemaTypeExample struct {
	Title      string
	Schema     *Schema
	AllValid   []string
	AllInvalid []string
}

func TestTypes(t *testing.T) {
	for _, example := range typeExamples {
		t.Run(example.Title, testType(t, example))
	}
}

func testType(t *testing.T, example schemaTypeExample) func(*testing.T) {
	return func(t *testing.T) {
		baseSchema := example.Schema
		for _, typ := range example.AllValid {
			schema := baseSchema.WithFormat(typ)
			err := schema.Validate(context.Background())
			require.NoError(t, err)
		}
		for _, typ := range example.AllInvalid {
			schema := baseSchema.WithFormat(typ)
			err := schema.Validate(context.Background())
			require.Error(t, err)
		}
	}
}

var typeExamples = []schemaTypeExample{
	{
		Title:  "STRING",
		Schema: NewStringSchema(),
		AllValid: []string{
			"",
			"byte",
			"binary",
			"date",
			"date-time",
			"password",
			// Not supported but allowed:
			"uri",
		},
		AllInvalid: []string{
			"code/golang",
		},
	},

	{
		Title:  "NUMBER",
		Schema: NewFloat64Schema(),
		AllValid: []string{
			"",
			"float",
			"double",
		},
		AllInvalid: []string{
			"f32",
		},
	},

	{
		Title:  "INTEGER",
		Schema: NewIntegerSchema(),
		AllValid: []string{
			"",
			"int32",
			"int64",
		},
		AllInvalid: []string{
			"uint8",
		},
	},
}

func TestSchemaErrors(t *testing.T) {
	for _, example := range schemaErrorExamples {
		t.Run(example.Title, testSchemaError(t, example))
	}
}

func testSchemaError(t *testing.T, example schemaErrorExample) func(*testing.T) {
	return func(t *testing.T) {
		msg := example.Error.Error()
		require.True(t, strings.Contains(msg, example.Want))
	}
}

type schemaErrorExample struct {
	Title string
	Error *SchemaError
	Want  string
}

var schemaErrorExamples = []schemaErrorExample{
	{
		Title: "SIMPLE",
		Error: &SchemaError{
			Value:  1,
			Schema: &Schema{},
			Reason: "SIMPLE",
		},
		Want: "SIMPLE",
	},
	{
		Title: "NEST",
		Error: &SchemaError{
			Value:  1,
			Schema: &Schema{},
			Reason: "PARENT",
			Origin: &SchemaError{
				Value:  1,
				Schema: &Schema{},
				Reason: "NEST",
			},
		},
		Want: "NEST",
	},
}

type schemaMultiErrorExample struct {
	Title          string
	Schema         *Schema
	Values         []interface{}
	ExpectedErrors []MultiError
}

func TestSchemasMultiError(t *testing.T) {
	for _, example := range schemaMultiErrorExamples {
		t.Run(example.Title, testSchemaMultiError(t, example))
	}
}

func testSchemaMultiError(t *testing.T, example schemaMultiErrorExample) func(*testing.T) {
	return func(t *testing.T) {
		schema := example.Schema
		for i, value := range example.Values {
			err := validateSchema(t, schema, value, MultiErrors())
			require.Error(t, err)
			require.IsType(t, MultiError{}, err)

			merr, _ := err.(MultiError)
			expected := example.ExpectedErrors[i]
			require.True(t, len(merr) > 0)
			require.Len(t, merr, len(expected))
			for _, e := range merr {
				require.IsType(t, &SchemaError{}, e)
				var found bool
				scherr, _ := e.(*SchemaError)
				for _, expectedErr := range expected {
					expectedScherr, _ := expectedErr.(*SchemaError)
					if reflect.DeepEqual(expectedScherr.reversePath, scherr.reversePath) &&
						expectedScherr.SchemaField == scherr.SchemaField {
						found = true
						break
					}
				}
				require.True(t, found, fmt.Sprintf("missing %s error on %s", scherr.SchemaField, strings.Join(scherr.JSONPointer(), ".")))
			}
		}
	}
}

var schemaMultiErrorExamples = []schemaMultiErrorExample{
	{
		Title: "STRING",
		Schema: NewStringSchema().
			WithMinLength(2).
			WithMaxLength(3).
			WithPattern("^[abc]+$"),
		Values: []interface{}{
			"f",
			"foobar",
		},
		ExpectedErrors: []MultiError{
			{&SchemaError{SchemaField: "minLength"}, &SchemaError{SchemaField: "pattern"}},
			{&SchemaError{SchemaField: "maxLength"}, &SchemaError{SchemaField: "pattern"}},
		},
	},
	{
		Title: "NUMBER",
		Schema: NewIntegerSchema().
			WithMin(1).
			WithMax(10),
		Values: []interface{}{
			0.5,
			10.1,
		},
		ExpectedErrors: []MultiError{
			{&SchemaError{SchemaField: "type"}, &SchemaError{SchemaField: "minimum"}},
			{&SchemaError{SchemaField: "type"}, &SchemaError{SchemaField: "maximum"}},
		},
	},
	{
		Title: "ARRAY: simple",
		Schema: NewArraySchema().
			WithMinItems(2).
			WithMaxItems(2).
			WithItems(NewStringSchema().
				WithPattern("^[abc]+$")),
		Values: []interface{}{
			[]interface{}{"foo"},
			[]interface{}{"foo", "bar", "fizz"},
		},
		ExpectedErrors: []MultiError{
			{
				&SchemaError{SchemaField: "minItems"},
				&SchemaError{SchemaField: "pattern", reversePath: []string{"0"}},
			},
			{
				&SchemaError{SchemaField: "maxItems"},
				&SchemaError{SchemaField: "pattern", reversePath: []string{"0"}},
				&SchemaError{SchemaField: "pattern", reversePath: []string{"1"}},
				&SchemaError{SchemaField: "pattern", reversePath: []string{"2"}},
			},
		},
	},
	{
		Title: "ARRAY: object",
		Schema: NewArraySchema().
			WithItems(NewObjectSchema().
				WithProperties(map[string]*Schema{
					"key1": NewStringSchema(),
					"key2": NewIntegerSchema(),
				}),
			),
		Values: []interface{}{
			[]interface{}{
				map[string]interface{}{
					"key1": 100, // not a string
					"key2": "not an integer",
				},
			},
		},
		ExpectedErrors: []MultiError{
			{
				&SchemaError{SchemaField: "type", reversePath: []string{"key1", "0"}},
				&SchemaError{SchemaField: "type", reversePath: []string{"key2", "0"}},
			},
		},
	},
	{
		Title: "OBJECT",
		Schema: NewObjectSchema().
			WithProperties(map[string]*Schema{
				"key1": NewStringSchema(),
				"key2": NewIntegerSchema(),
				"key3": NewArraySchema().
					WithItems(NewStringSchema().
						WithPattern("^[abc]+$")),
			}),
		Values: []interface{}{
			map[string]interface{}{
				"key1": 100, // not a string
				"key2": "not an integer",
				"key3": []interface{}{"abc", "def"},
			},
		},
		ExpectedErrors: []MultiError{
			{
				&SchemaError{SchemaField: "type", reversePath: []string{"key1"}},
				&SchemaError{SchemaField: "type", reversePath: []string{"key2"}},
				&SchemaError{SchemaField: "pattern", reversePath: []string{"1", "key3"}},
			},
		},
	},
}

func TestIssue283(t *testing.T) {
	const api = `
openapi: "3.0.1"
components:
  schemas:
    Test:
      properties:
        name:
          type: string
        ownerName:
          not:
            type: boolean
      type: object
`
	data := map[string]interface{}{
		"name":      "kin-openapi",
		"ownerName": true,
	}
	s, err := NewLoader().LoadFromData([]byte(api))
	require.NoError(t, err)
	require.NotNil(t, s)
	err = s.Components.Schemas["Test"].Value.VisitJSON(data)
	require.NotNil(t, err)
	require.NotEqual(t, errSchema, err)
	require.Contains(t, err.Error(), `Error at "/ownerName": Doesn't match schema "not"`)
}

func TestValidationFailsOnInvalidPattern(t *testing.T) {
	schema := Schema{
		Pattern: "[",
		Type:    "string",
	}

	var err = schema.Validate(context.Background())
	require.Error(t, err)
}
