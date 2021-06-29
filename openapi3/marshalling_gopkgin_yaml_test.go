package openapi3

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v2"
)

func TestIssue241Gopkg(t *testing.T) {
	const spec = `components:
  schemas:
    FooBar:
      properties:
        type_url:
          type: string
        value:
          format: byte
          type: string
      type: object
info:
  title: sample
  version: version not set
openapi: 3.0.3
paths: {}
`

	doc, err := NewSwaggerLoader().LoadSwaggerFromData([]byte(spec))
	require.NoError(t, err)
	err = doc.Validate(context.Background())
	require.NoError(t, err)

	yml, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.Equal(t, spec, string(yml))
}
