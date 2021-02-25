package openapi3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"strconv"
	"strings"

	"github.com/ghodss/yaml"
)

func foundUnresolvedRef(ref string) error {
	return fmt.Errorf("found unresolved ref: %q", ref)
}

func failedToResolveRefFragmentPart(value, what string) error {
	return fmt.Errorf("failed to resolve %q in fragment in URI: %q", what, value)
}

// SwaggerLoader helps deserialize a Swagger object
type SwaggerLoader struct {
	// IsExternalRefsAllowed enables visiting other files
	IsExternalRefsAllowed bool

	// LoadSwaggerFromURIFunc allows overriding the file/URL reading func
	LoadSwaggerFromURIFunc func(loader *SwaggerLoader, url *url.URL) (*Swagger, error)

	Context context.Context

	visited      map[interface{}]struct{}
	visitedFiles map[string]struct{}
}

// NewSwaggerLoader returns an empty SwaggerLoader
func NewSwaggerLoader() *SwaggerLoader {
	return &SwaggerLoader{}
}

func (swaggerLoader *SwaggerLoader) reset() {
	swaggerLoader.visitedFiles = make(map[string]struct{})
}

// LoadSwaggerFromURI loads a spec from a remote URL
func (swaggerLoader *SwaggerLoader) LoadSwaggerFromURI(location *url.URL) (*Swagger, error) {
	swaggerLoader.reset()
	return swaggerLoader.loadSwaggerFromURIInternal(location)
}

func (swaggerLoader *SwaggerLoader) loadSwaggerFromURIInternal(location *url.URL) (*Swagger, error) {
	f := swaggerLoader.LoadSwaggerFromURIFunc
	if f != nil {
		return f(swaggerLoader, location)
	}
	data, err := readURL(location)
	if err != nil {
		return nil, err
	}
	return swaggerLoader.loadSwaggerFromDataWithPathInternal(data, location)
}

// loadSingleElementFromURI read the data from ref and unmarshal to JSON to the
// passed element.
func (swaggerLoader *SwaggerLoader) loadSingleElementFromURI(ref string, rootPath *url.URL, element json.Unmarshaler) error {
	if !swaggerLoader.IsExternalRefsAllowed {
		return fmt.Errorf("encountered non-allowed external reference: %q", ref)
	}

	parsedURL, err := url.Parse(ref)
	if err != nil {
		return err
	}

	if parsedURL.Fragment != "" {
		return errors.New("references to files which contain more than one element definition are not supported")
	}

	resolvedPath, err := resolvePath(rootPath, parsedURL)
	if err != nil {
		return fmt.Errorf("could not resolve path: %v", err)
	}

	data, err := readURL(resolvedPath)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, element); err != nil {
		return err
	}

	return nil
}

func readURL(location *url.URL) ([]byte, error) {
	if location.Scheme != "" && location.Host != "" {
		resp, err := http.Get(location.String())
		if err != nil {
			return nil, err
		}
		data, err := ioutil.ReadAll(resp.Body)
		defer resp.Body.Close()
		if err != nil {
			return nil, err
		}
		return data, nil
	}
	if location.Scheme != "" || location.Host != "" || location.RawQuery != "" {
		return nil, fmt.Errorf("unsupported URI: %q", location.String())
	}
	data, err := ioutil.ReadFile(location.Path)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// LoadSwaggerFromFile loads a spec from a local file path
func (swaggerLoader *SwaggerLoader) LoadSwaggerFromFile(path string) (*Swagger, error) {
	swaggerLoader.reset()
	return swaggerLoader.loadSwaggerFromFileInternal(path)
}

func (swaggerLoader *SwaggerLoader) loadSwaggerFromFileInternal(path string) (*Swagger, error) {
	f := swaggerLoader.LoadSwaggerFromURIFunc
	pathAsURL := &url.URL{Path: path}
	if f != nil {
		x, err := f(swaggerLoader, pathAsURL)
		return x, err
	}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return swaggerLoader.loadSwaggerFromDataWithPathInternal(data, pathAsURL)
}

// LoadSwaggerFromData loads a spec from a byte array
func (swaggerLoader *SwaggerLoader) LoadSwaggerFromData(data []byte) (*Swagger, error) {
	swaggerLoader.reset()
	return swaggerLoader.loadSwaggerFromDataInternal(data)
}

func (swaggerLoader *SwaggerLoader) loadSwaggerFromDataInternal(data []byte) (*Swagger, error) {
	swagger := &Swagger{}
	if err := yaml.Unmarshal(data, swagger); err != nil {
		return nil, err
	}
	return swagger, swaggerLoader.ResolveRefsIn(swagger, nil)
}

// LoadSwaggerFromDataWithPath takes the OpenApi spec data in bytes and a path where the resolver can find referred
// elements and returns a *Swagger with all resolved data or an error if unable to load data or resolve refs.
func (swaggerLoader *SwaggerLoader) LoadSwaggerFromDataWithPath(data []byte, path *url.URL) (*Swagger, error) {
	swaggerLoader.reset()
	return swaggerLoader.loadSwaggerFromDataWithPathInternal(data, path)
}

func (swaggerLoader *SwaggerLoader) loadSwaggerFromDataWithPathInternal(data []byte, path *url.URL) (*Swagger, error) {
	swagger := &Swagger{}
	if err := yaml.Unmarshal(data, swagger); err != nil {
		return nil, err
	}
	return swagger, swaggerLoader.ResolveRefsIn(swagger, path)
}

// ResolveRefsIn expands references if for instance spec was just unmarshalled
func (swaggerLoader *SwaggerLoader) ResolveRefsIn(swagger *Swagger, path *url.URL) (err error) {
	if swaggerLoader.visited == nil {
		swaggerLoader.visited = make(map[interface{}]struct{})
	}
	if swaggerLoader.visitedFiles == nil {
		swaggerLoader.reset()
	}

	// Visit all components
	components := swagger.Components
	for _, component := range components.Headers {
		if err = swaggerLoader.resolveHeaderRef(swagger, component, path); err != nil {
			return
		}
	}
	for _, component := range components.Parameters {
		if err = swaggerLoader.resolveParameterRef(swagger, component, path); err != nil {
			return
		}
	}
	for _, component := range components.RequestBodies {
		if err = swaggerLoader.resolveRequestBodyRef(swagger, component, path); err != nil {
			return
		}
	}
	for _, component := range components.Responses {
		if err = swaggerLoader.resolveResponseRef(swagger, component, path); err != nil {
			return
		}
	}
	for _, component := range components.Schemas {
		if err = swaggerLoader.resolveSchemaRef(swagger, component, path); err != nil {
			return
		}
	}
	for _, component := range components.SecuritySchemes {
		if err = swaggerLoader.resolveSecuritySchemeRef(swagger, component, path); err != nil {
			return
		}
	}
	for _, component := range components.Examples {
		if err = swaggerLoader.resolveExampleRef(swagger, component, path); err != nil {
			return
		}
	}

	// Visit all operations
	for entrypoint, pathItem := range swagger.Paths {
		if pathItem == nil {
			continue
		}
		if err = swaggerLoader.resolvePathItemRef(swagger, entrypoint, pathItem, path); err != nil {
			return
		}
	}

	return
}

func copyURL(basePath *url.URL) (*url.URL, error) {
	return url.Parse(basePath.String())
}

func join(basePath *url.URL, relativePath *url.URL) (*url.URL, error) {
	if basePath == nil {
		return relativePath, nil
	}
	newPath, err := copyURL(basePath)
	if err != nil {
		return nil, fmt.Errorf("cannot copy path: %q", basePath.String())
	}
	newPath.Path = path.Join(path.Dir(newPath.Path), relativePath.Path)
	return newPath, nil
}

func resolvePath(basePath *url.URL, componentPath *url.URL) (*url.URL, error) {
	if componentPath.Scheme == "" && componentPath.Host == "" {
		// support absolute paths
		if componentPath.Path[0] == '/' {
			return componentPath, nil
		}
		return join(basePath, componentPath)
	}
	return componentPath, nil
}

func isSingleRefElement(ref string) bool {
	return !strings.Contains(ref, "#")
}

func (swaggerLoader *SwaggerLoader) resolveComponent(
	swagger *Swagger,
	ref string,
	path *url.URL,
	resolved interface{},
) (
	componentPath *url.URL,
	err error,
) {
	if swagger, ref, componentPath, err = swaggerLoader.resolveRefSwagger(swagger, ref, path); err != nil {
		return nil, err
	}

	parsedURL, err := url.Parse(ref)
	if err != nil {
		return nil, fmt.Errorf("cannot parse reference: %q: %v", ref, parsedURL)
	}
	fragment := parsedURL.Fragment
	if !strings.HasPrefix(fragment, "/") {
		return nil, fmt.Errorf("expected fragment prefix '#/' in URI %q", ref)
	}

	var cursor interface{}
	cursor = swagger
	for _, pathPart := range strings.Split(fragment[1:], "/") {
		pathPart = unescapeRefString(pathPart)

		if cursor, err = drillIntoSwaggerField(cursor, pathPart); err != nil {
			e := failedToResolveRefFragmentPart(ref, pathPart)
			return nil, fmt.Errorf("%s: %s", e.Error(), err.Error())
		}
		if cursor == nil {
			return nil, failedToResolveRefFragmentPart(ref, pathPart)
		}
	}

	switch {
	case reflect.TypeOf(cursor) == reflect.TypeOf(resolved):
		reflect.ValueOf(resolved).Elem().Set(reflect.ValueOf(cursor).Elem())
		return componentPath, nil

	case reflect.TypeOf(cursor) == reflect.TypeOf(map[string]interface{}{}):
		codec := func(got, expect interface{}) error {
			enc, err := json.Marshal(got)
			if err != nil {
				return err
			}
			if err = json.Unmarshal(enc, expect); err != nil {
				return err
			}
			return nil
		}
		if err := codec(cursor, resolved); err != nil {
			return nil, fmt.Errorf("bad data in %q", ref)
		}
		return componentPath, nil

	default:
		return nil, fmt.Errorf("bad data in %q", ref)
	}
}

func drillIntoSwaggerField(cursor interface{}, fieldName string) (interface{}, error) {
	switch val := reflect.Indirect(reflect.ValueOf(cursor)); val.Kind() {
	case reflect.Map:
		elementValue := val.MapIndex(reflect.ValueOf(fieldName))
		if !elementValue.IsValid() {
			return nil, fmt.Errorf("map key %q not found", fieldName)
		}
		return elementValue.Interface(), nil

	case reflect.Slice:
		i, err := strconv.ParseUint(fieldName, 10, 32)
		if err != nil {
			return nil, err
		}
		index := int(i)
		if 0 > index || index >= val.Len() {
			return nil, errors.New("slice index out of bounds")
		}
		return val.Index(index).Interface(), nil

	case reflect.Struct:
		hasFields := false
		for i := 0; i < val.NumField(); i++ {
			hasFields = true
			field := val.Type().Field(i)
			tagValue := field.Tag.Get("yaml")
			yamlKey := strings.Split(tagValue, ",")[0]
			if yamlKey == fieldName {
				return val.Field(i).Interface(), nil
			}
		}
		// if cursor is a "ref wrapper" struct (e.g. RequestBodyRef),
		if _, ok := val.Type().FieldByName("Value"); ok {
			// try digging into its Value field
			return drillIntoSwaggerField(val.FieldByName("Value").Interface(), fieldName)
		}
		if hasFields {
			if ff := val.Type().Field(0); ff.PkgPath == "" && ff.Name == "ExtensionProps" {
				extensions := val.Field(0).Interface().(ExtensionProps).Extensions
				if enc, ok := extensions[fieldName]; ok {
					var dec interface{}
					if err := json.Unmarshal(enc.(json.RawMessage), &dec); err != nil {
						return nil, err
					}
					return dec, nil
				}
			}
		}
		return nil, fmt.Errorf("struct field %q not found", fieldName)

	default:
		return nil, errors.New("not a map, slice nor struct")
	}
}

func (swaggerLoader *SwaggerLoader) resolveRefSwagger(swagger *Swagger, ref string, path *url.URL) (*Swagger, string, *url.URL, error) {
	componentPath := path
	if !strings.HasPrefix(ref, "#") {
		if !swaggerLoader.IsExternalRefsAllowed {
			return nil, "", nil, fmt.Errorf("encountered non-allowed external reference: %q", ref)
		}
		parsedURL, err := url.Parse(ref)
		if err != nil {
			return nil, "", nil, fmt.Errorf("cannot parse reference: %q: %v", ref, parsedURL)
		}
		fragment := parsedURL.Fragment
		parsedURL.Fragment = ""

		resolvedPath, err := resolvePath(path, parsedURL)
		if err != nil {
			return nil, "", nil, fmt.Errorf("error resolving path: %v", err)
		}

		if swagger, err = swaggerLoader.loadSwaggerFromURIInternal(resolvedPath); err != nil {
			return nil, "", nil, fmt.Errorf("error resolving reference %q: %v", ref, err)
		}
		ref = "#" + fragment
		componentPath = resolvedPath
	}
	return swagger, ref, componentPath, nil
}

func (swaggerLoader *SwaggerLoader) resolveHeaderRef(swagger *Swagger, component *HeaderRef, documentPath *url.URL) error {
	visited := swaggerLoader.visited
	if _, isVisited := visited[component]; isVisited {
		return nil
	}
	if component != nil && component.Value != nil {
		for c := range visited {
			if x, ok := c.(*HeaderRef); ok && x.Value == component.Value {
				return nil
			}
		}
	}
	visited[component] = struct{}{}

	const prefix = "#/components/headers/"
	if component == nil {
		return errors.New("invalid header: value MUST be a JSON object")
	}
	if ref := component.Ref; ref != "" {
		if isSingleRefElement(ref) {
			var header Header
			if err := swaggerLoader.loadSingleElementFromURI(ref, documentPath, &header); err != nil {
				return err
			}

			component.Value = &header
		} else {
			var resolved HeaderRef
			componentPath, err := swaggerLoader.resolveComponent(swagger, ref, documentPath, &resolved)
			if err != nil {
				return err
			}
			if err := swaggerLoader.resolveHeaderRef(swagger, &resolved, componentPath); err != nil {
				return err
			}
			component.Value = resolved.Value
		}
	}
	value := component.Value
	if value == nil {
		return nil
	}
	if schema := value.Schema; schema != nil {
		if err := swaggerLoader.resolveSchemaRef(swagger, schema, documentPath); err != nil {
			return err
		}
	}
	return nil
}

func (swaggerLoader *SwaggerLoader) resolveParameterRef(swagger *Swagger, component *ParameterRef, documentPath *url.URL) error {
	visited := swaggerLoader.visited
	if _, isVisited := visited[component]; isVisited {
		return nil
	}
	if component != nil && component.Value != nil {
		for c := range visited {
			if x, ok := c.(*ParameterRef); ok && x.Value == component.Value {
				return nil
			}
		}
	}
	visited[component] = struct{}{}

	const prefix = "#/components/parameters/"
	if component == nil {
		return errors.New("invalid parameter: value MUST be a JSON object")
	}
	ref := component.Ref
	if ref != "" {
		if isSingleRefElement(ref) {
			var param Parameter
			if err := swaggerLoader.loadSingleElementFromURI(ref, documentPath, &param); err != nil {
				return err
			}
			component.Value = &param
		} else {
			var resolved ParameterRef
			componentPath, err := swaggerLoader.resolveComponent(swagger, ref, documentPath, &resolved)
			if err != nil {
				return err
			}
			if err := swaggerLoader.resolveParameterRef(swagger, &resolved, componentPath); err != nil {
				return err
			}
			component.Value = resolved.Value
		}
	}
	value := component.Value
	if value == nil {
		return nil
	}

	refDocumentPath, err := referencedDocumentPath(documentPath, ref)
	if err != nil {
		return err
	}

	if value.Content != nil && value.Schema != nil {
		return errors.New("cannot contain both schema and content in a parameter")
	}
	for _, contentType := range value.Content {
		if schema := contentType.Schema; schema != nil {
			if err := swaggerLoader.resolveSchemaRef(swagger, schema, refDocumentPath); err != nil {
				return err
			}
		}
	}
	if schema := value.Schema; schema != nil {
		if err := swaggerLoader.resolveSchemaRef(swagger, schema, refDocumentPath); err != nil {
			return err
		}
	}
	return nil
}

func (swaggerLoader *SwaggerLoader) resolveRequestBodyRef(swagger *Swagger, component *RequestBodyRef, documentPath *url.URL) error {
	visited := swaggerLoader.visited
	if _, isVisited := visited[component]; isVisited {
		return nil
	}
	if component != nil && component.Value != nil {
		for c := range visited {
			if x, ok := c.(*RequestBodyRef); ok && x.Value == component.Value {
				return nil
			}
		}
	}
	visited[component] = struct{}{}

	const prefix = "#/components/requestBodies/"
	if component == nil {
		return errors.New("invalid requestBody: value MUST be a JSON object")
	}
	if ref := component.Ref; ref != "" {
		if isSingleRefElement(ref) {
			var requestBody RequestBody
			if err := swaggerLoader.loadSingleElementFromURI(ref, documentPath, &requestBody); err != nil {
				return err
			}

			component.Value = &requestBody
		} else {
			var resolved RequestBodyRef
			componentPath, err := swaggerLoader.resolveComponent(swagger, ref, documentPath, &resolved)
			if err != nil {
				return err
			}
			if err = swaggerLoader.resolveRequestBodyRef(swagger, &resolved, componentPath); err != nil {
				return err
			}
			component.Value = resolved.Value
		}
	}
	value := component.Value
	if value == nil {
		return nil
	}
	for _, contentType := range value.Content {
		for name, example := range contentType.Examples {
			if err := swaggerLoader.resolveExampleRef(swagger, example, documentPath); err != nil {
				return err
			}
			contentType.Examples[name] = example
		}
		if schema := contentType.Schema; schema != nil {
			if err := swaggerLoader.resolveSchemaRef(swagger, schema, documentPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func (swaggerLoader *SwaggerLoader) resolveResponseRef(swagger *Swagger, component *ResponseRef, documentPath *url.URL) error {
	visited := swaggerLoader.visited
	if _, isVisited := visited[component]; isVisited {
		return nil
	}
	if component != nil && component.Value != nil {
		for c := range visited {
			if x, ok := c.(*ResponseRef); ok && x.Value == component.Value {
				return nil
			}
		}
	}
	visited[component] = struct{}{}

	const prefix = "#/components/responses/"
	if component == nil {
		return errors.New("invalid response: value MUST be a JSON object")
	}
	ref := component.Ref
	if ref != "" {
		if isSingleRefElement(ref) {
			var resp Response
			if err := swaggerLoader.loadSingleElementFromURI(ref, documentPath, &resp); err != nil {
				return err
			}
			component.Value = &resp
		} else {
			var resolved ResponseRef
			componentPath, err := swaggerLoader.resolveComponent(swagger, ref, documentPath, &resolved)
			if err != nil {
				return err
			}
			if err := swaggerLoader.resolveResponseRef(swagger, &resolved, componentPath); err != nil {
				return err
			}
			component.Value = resolved.Value
		}
	}
	refDocumentPath, err := referencedDocumentPath(documentPath, ref)
	if err != nil {
		return err
	}

	value := component.Value
	if value == nil {
		return nil
	}
	for _, header := range value.Headers {
		if err := swaggerLoader.resolveHeaderRef(swagger, header, refDocumentPath); err != nil {
			return err
		}
	}
	for _, contentType := range value.Content {
		if contentType == nil {
			continue
		}
		for name, example := range contentType.Examples {
			if err := swaggerLoader.resolveExampleRef(swagger, example, refDocumentPath); err != nil {
				return err
			}
			contentType.Examples[name] = example
		}
		if schema := contentType.Schema; schema != nil {
			if err := swaggerLoader.resolveSchemaRef(swagger, schema, refDocumentPath); err != nil {
				return err
			}
			contentType.Schema = schema
		}
	}
	for _, link := range value.Links {
		if err := swaggerLoader.resolveLinkRef(swagger, link, refDocumentPath); err != nil {
			return err
		}
	}
	return nil
}

func (swaggerLoader *SwaggerLoader) resolveSchemaRef(swagger *Swagger, component *SchemaRef, documentPath *url.URL) error {
	visited := swaggerLoader.visited
	if _, isVisited := visited[component]; isVisited {
		return nil
	}
	if component != nil && component.Value != nil {
		for c := range visited {
			if x, ok := c.(*SchemaRef); ok && x.Value == component.Value {
				return nil
			}
		}
	}
	visited[component] = struct{}{}

	const prefix = "#/components/schemas/"
	if component == nil {
		return errors.New("invalid schema: value MUST be a JSON object")
	}
	ref := component.Ref
	if ref != "" {
		if isSingleRefElement(ref) {
			var schema Schema
			if err := swaggerLoader.loadSingleElementFromURI(ref, documentPath, &schema); err != nil {
				return err
			}
			component.Value = &schema
		} else {
			var resolved SchemaRef
			componentPath, err := swaggerLoader.resolveComponent(swagger, ref, documentPath, &resolved)
			if err != nil {
				return err
			}
			if err := swaggerLoader.resolveSchemaRef(swagger, &resolved, componentPath); err != nil {
				return err
			}
			component.Value = resolved.Value
		}
	}

	refDocumentPath, err := referencedDocumentPath(documentPath, ref)
	if err != nil {
		return err
	}

	value := component.Value
	if value == nil {
		return nil
	}

	// ResolveRefs referred schemas
	if v := value.Items; v != nil {
		if err := swaggerLoader.resolveSchemaRef(swagger, v, refDocumentPath); err != nil {
			return err
		}
	}
	for _, v := range value.Properties {
		if err := swaggerLoader.resolveSchemaRef(swagger, v, refDocumentPath); err != nil {
			return err
		}
	}
	if v := value.AdditionalProperties; v != nil {
		if err := swaggerLoader.resolveSchemaRef(swagger, v, refDocumentPath); err != nil {
			return err
		}
	}
	if v := value.Not; v != nil {
		if err := swaggerLoader.resolveSchemaRef(swagger, v, refDocumentPath); err != nil {
			return err
		}
	}
	for _, v := range value.AllOf {
		if err := swaggerLoader.resolveSchemaRef(swagger, v, refDocumentPath); err != nil {
			return err
		}
	}
	for _, v := range value.AnyOf {
		if err := swaggerLoader.resolveSchemaRef(swagger, v, refDocumentPath); err != nil {
			return err
		}
	}
	for _, v := range value.OneOf {
		if err := swaggerLoader.resolveSchemaRef(swagger, v, refDocumentPath); err != nil {
			return err
		}
	}

	return nil
}

func (swaggerLoader *SwaggerLoader) resolveSecuritySchemeRef(swagger *Swagger, component *SecuritySchemeRef, documentPath *url.URL) error {
	visited := swaggerLoader.visited
	if _, isVisited := visited[component]; isVisited {
		return nil
	}
	if component != nil && component.Value != nil {
		for c := range visited {
			if x, ok := c.(*SecuritySchemeRef); ok && x.Value == component.Value {
				return nil
			}
		}
	}
	visited[component] = struct{}{}

	const prefix = "#/components/securitySchemes/"
	if component == nil {
		return errors.New("invalid securityScheme: value MUST be a JSON object")
	}
	if ref := component.Ref; ref != "" {
		if isSingleRefElement(ref) {
			var scheme SecurityScheme
			if err := swaggerLoader.loadSingleElementFromURI(ref, documentPath, &scheme); err != nil {
				return err
			}

			component.Value = &scheme
		} else {
			var resolved SecuritySchemeRef
			componentPath, err := swaggerLoader.resolveComponent(swagger, ref, documentPath, &resolved)
			if err != nil {
				return err
			}
			if err := swaggerLoader.resolveSecuritySchemeRef(swagger, &resolved, componentPath); err != nil {
				return err
			}
			component.Value = resolved.Value
		}
	}
	return nil
}

func (swaggerLoader *SwaggerLoader) resolveExampleRef(swagger *Swagger, component *ExampleRef, documentPath *url.URL) error {
	visited := swaggerLoader.visited
	if _, isVisited := visited[component]; isVisited {
		return nil
	}
	if component != nil && component.Value != nil {
		for c := range visited {
			if x, ok := c.(*ExampleRef); ok && x.Value == component.Value {
				return nil
			}
		}
	}
	visited[component] = struct{}{}

	const prefix = "#/components/examples/"
	if component == nil {
		return errors.New("invalid example: value MUST be a JSON object")
	}
	if ref := component.Ref; ref != "" {
		if isSingleRefElement(ref) {
			var example Example
			if err := swaggerLoader.loadSingleElementFromURI(ref, documentPath, &example); err != nil {
				return err
			}

			component.Value = &example
		} else {
			var resolved ExampleRef
			componentPath, err := swaggerLoader.resolveComponent(swagger, ref, documentPath, &resolved)
			if err != nil {
				return err
			}
			if err := swaggerLoader.resolveExampleRef(swagger, &resolved, componentPath); err != nil {
				return err
			}
			component.Value = resolved.Value
		}
	}
	return nil
}

func (swaggerLoader *SwaggerLoader) resolveLinkRef(swagger *Swagger, component *LinkRef, documentPath *url.URL) error {
	visited := swaggerLoader.visited
	if _, isVisited := visited[component]; isVisited {
		return nil
	}
	if component != nil && component.Value != nil {
		for c := range visited {
			if x, ok := c.(*LinkRef); ok && x.Value == component.Value {
				return nil
			}
		}
	}
	visited[component] = struct{}{}

	const prefix = "#/components/links/"
	if component == nil {
		return errors.New("invalid link: value MUST be a JSON object")
	}
	if ref := component.Ref; ref != "" {
		if isSingleRefElement(ref) {
			var link Link
			if err := swaggerLoader.loadSingleElementFromURI(ref, documentPath, &link); err != nil {
				return err
			}

			component.Value = &link
		} else {
			var resolved LinkRef
			componentPath, err := swaggerLoader.resolveComponent(swagger, ref, documentPath, &resolved)
			if err != nil {
				return err
			}
			if err := swaggerLoader.resolveLinkRef(swagger, &resolved, componentPath); err != nil {
				return err
			}
			component.Value = resolved.Value
		}
	}
	return nil
}

func (swaggerLoader *SwaggerLoader) resolvePathItemRef(swagger *Swagger, entrypoint string, pathItem *PathItem, documentPath *url.URL) (err error) {
	visited := swaggerLoader.visitedFiles
	key := "_"
	if documentPath != nil {
		key = documentPath.EscapedPath()
	}
	key += entrypoint
	if _, isVisited := visited[key]; isVisited {
		return nil
	}
	visited[key] = struct{}{}

	const prefix = "#/paths/"
	if pathItem == nil {
		return errors.New("invalid path item: value MUST be a JSON object")
	}
	ref := pathItem.Ref
	if ref != "" {
		if isSingleRefElement(ref) {
			var p PathItem
			if err := swaggerLoader.loadSingleElementFromURI(ref, documentPath, &p); err != nil {
				return err
			}
			*pathItem = p
		} else {
			if swagger, ref, documentPath, err = swaggerLoader.resolveRefSwagger(swagger, ref, documentPath); err != nil {
				return
			}

			if !strings.HasPrefix(ref, prefix) {
				err = fmt.Errorf("expected prefix %q in URI %q", prefix, ref)
				return
			}
			id := unescapeRefString(ref[len(prefix):])

			definitions := swagger.Paths
			if definitions == nil {
				return failedToResolveRefFragmentPart(ref, "paths")
			}
			resolved := definitions[id]
			if resolved == nil {
				return failedToResolveRefFragmentPart(ref, id)
			}

			*pathItem = *resolved
		}
	}

	refDocumentPath, err := referencedDocumentPath(documentPath, ref)
	if err != nil {
		return err
	}

	for _, parameter := range pathItem.Parameters {
		if err = swaggerLoader.resolveParameterRef(swagger, parameter, refDocumentPath); err != nil {
			return
		}
	}
	for _, operation := range pathItem.Operations() {
		for _, parameter := range operation.Parameters {
			if err = swaggerLoader.resolveParameterRef(swagger, parameter, refDocumentPath); err != nil {
				return
			}
		}
		if requestBody := operation.RequestBody; requestBody != nil {
			if err = swaggerLoader.resolveRequestBodyRef(swagger, requestBody, refDocumentPath); err != nil {
				return
			}
		}
		for _, response := range operation.Responses {
			if err = swaggerLoader.resolveResponseRef(swagger, response, refDocumentPath); err != nil {
				return
			}
		}
	}

	return nil
}

func unescapeRefString(ref string) string {
	return strings.Replace(strings.Replace(ref, "~1", "/", -1), "~0", "~", -1)
}

func referencedDocumentPath(documentPath *url.URL, ref string) (*url.URL, error) {
	if documentPath == nil {
		return nil, nil
	}

	newDocumentPath, err := copyURL(documentPath)
	if err != nil {
		return nil, err
	}
	refPath, err := url.Parse(ref)
	if err != nil {
		return nil, err
	}
	newDocumentPath.Path = path.Join(path.Dir(newDocumentPath.Path), path.Dir(refPath.Path)) + "/"

	return newDocumentPath, nil
}
