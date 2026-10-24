package openapi

import (
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// fromKinDoc translates a loaded kin-openapi document into this package's
// internal model. kin-openapi has already resolved local $refs, so every
// *Ref here carries its Value; this function only copies the parts the
// adapter uses.
func fromKinDoc(k *openapi3.T) (doc, error) {
	d := doc{Paths: map[string]map[string]operation{}}
	if k.Info != nil {
		d.Title = k.Info.Title
		d.Version = k.Info.Version
	}
	if err := readDocExtensions(k.Extensions, &d); err != nil {
		return doc{}, err
	}
	if k.Paths == nil {
		return d, nil
	}
	schemas := newSchemaTranslator()
	for path, item := range k.Paths.Map() {
		if item == nil {
			continue
		}
		ops, err := fromKinPathItem(k, path, item, schemas)
		if err != nil {
			return doc{}, err
		}
		d.Paths[path] = ops
	}
	return d, nil
}

// readDocExtensions reads the document-level x-mimic-* extensions into d.
func readDocExtensions(ext map[string]any, d *doc) error {
	if _, err := decodeExtension(ext, "x-mimic-name", &d.MimicName); err != nil {
		return err
	}
	var persist bool
	ok, err := decodeExtension(ext, "x-mimic-persist", &persist)
	if err != nil {
		return err
	}
	if ok {
		d.MimicPersist = &persist
	}
	var validation string
	ok, err = decodeExtension(ext, "x-mimic-validation", &validation)
	if err != nil || !ok {
		return err
	}
	mode, err := ParseValidationMode(validation)
	if err != nil {
		return fmt.Errorf("x-mimic-validation: %w", err)
	}
	d.MimicValidation = &mode
	return nil
}

// fromKinPathItem translates every operation of one path, attaching each
// operation's request validator.
func fromKinPathItem(k *openapi3.T, path string, item *openapi3.PathItem, schemas *schemaTranslator) (map[string]operation, error) {
	ops := map[string]operation{}
	for method, kop := range item.Operations() {
		op, err := fromKinOperation(kop, item.Parameters, schemas)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", strings.ToUpper(method), path, err)
		}
		var pathParams []string
		for _, p := range op.Parameters {
			if p.In == openapi3.ParameterInPath {
				pathParams = append(pathParams, p.Name)
			}
		}
		op.validate = newRequestValidator(k, path, strings.ToUpper(method), item, kop, pathParams)
		ops[strings.ToLower(method)] = op
	}
	return ops, nil
}

func fromKinOperation(k *openapi3.Operation, pathParams openapi3.Parameters, schemas *schemaTranslator) (operation, error) {
	op := operation{OperationID: k.OperationID, Responses: map[string]response{}}

	var b behaviorSpec
	ok, err := decodeExtension(k.Extensions, "x-mimic-behavior", &b)
	if err != nil {
		return operation{}, err
	}
	if ok {
		op.Behavior = &b
	}

	op.Parameters = mergeParameters(k.Parameters, pathParams, schemas)

	if k.RequestBody != nil && k.RequestBody.Value != nil {
		op.RequestBody = &requestBody{
			Required: k.RequestBody.Value.Required,
			Content:  fromKinContent(k.RequestBody.Value.Content, schemas),
		}
	}

	if k.Responses != nil {
		for code, ref := range k.Responses.Map() {
			if ref != nil && ref.Value != nil {
				op.Responses[code] = response{Content: fromKinContent(ref.Value.Content, schemas)}
			}
		}
	}
	return op, nil
}

// mergeParameters combines an operation's parameters with its path item's:
// path-item-level parameters apply to every operation unless the operation
// redeclares the same name+location.
func mergeParameters(opParams, pathParams openapi3.Parameters, schemas *schemaTranslator) []parameter {
	var out []parameter
	seen := map[string]bool{}
	for _, ref := range append(append(openapi3.Parameters{}, opParams...), pathParams...) {
		if ref == nil || ref.Value == nil {
			continue
		}
		p := ref.Value
		key := p.In + ":" + p.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, parameter{
			Name:     p.Name,
			In:       p.In,
			Required: p.Required,
			Schema:   schemas.translate(p.Schema),
		})
	}
	return out
}

func fromKinContent(c openapi3.Content, schemas *schemaTranslator) map[string]mediaType {
	if len(c) == 0 {
		return nil
	}
	out := make(map[string]mediaType, len(c))
	for ct, mt := range c {
		if mt == nil {
			continue
		}
		m := mediaType{Schema: schemas.translate(mt.Schema), Example: mt.Example}
		for name, ex := range mt.Examples {
			if ex == nil || ex.Value == nil {
				continue
			}
			if m.Examples == nil {
				m.Examples = map[string]example{}
			}
			m.Examples[name] = example{Value: ex.Value.Value}
		}
		out[ct] = m
	}
	return out
}

// schemaTranslator memoizes kin schema → internal schema so a schema
// referenced from several places is translated once, and a recursive
// schema becomes a cyclic graph instead of infinite recursion.
type schemaTranslator struct {
	done map[*openapi3.Schema]*schema
}

func newSchemaTranslator() *schemaTranslator {
	return &schemaTranslator{done: map[*openapi3.Schema]*schema{}}
}

func (t *schemaTranslator) translate(ref *openapi3.SchemaRef) *schema {
	if ref == nil || ref.Value == nil {
		return nil
	}
	k := ref.Value
	if s, ok := t.done[k]; ok {
		return s
	}
	s := &schema{Format: k.Format, Example: k.Example, Enum: k.Enum, Type: firstNonNullType(k.Type)}
	t.done[k] = s
	s.Properties = t.translateProperties(k.Properties)
	s.Items = t.translate(k.Items)

	for _, ref := range k.AllOf {
		mergeAllOf(s, t.translate(ref))
	}
	s.Variants = append(s.Variants, t.translateAll(k.OneOf)...)
	s.Variants = append(s.Variants, t.translateAll(k.AnyOf)...)
	if s.Type == "" && len(s.Properties) > 0 {
		s.Type = "object"
	}
	return s
}

// firstNonNullType picks the schema's type, ignoring OpenAPI 3.1's "null"
// in a type list.
func firstNonNullType(types *openapi3.Types) string {
	if types == nil {
		return ""
	}
	for _, typ := range types.Slice() {
		if typ != "null" {
			return typ
		}
	}
	return ""
}

func (t *schemaTranslator) translateProperties(props openapi3.Schemas) map[string]*schema {
	if len(props) == 0 {
		return nil
	}
	out := make(map[string]*schema, len(props))
	for name, p := range props {
		out[name] = t.translate(p)
	}
	return out
}

// translateAll translates a list of alternatives, dropping empty ones.
func (t *schemaTranslator) translateAll(refs openapi3.SchemaRefs) []*schema {
	var out []*schema
	for _, ref := range refs {
		if v := t.translate(ref); v != nil {
			out = append(out, v)
		}
	}
	return out
}

// mergeAllOf folds one allOf member into s. Fields s already declares win;
// properties are unioned. A member still being translated (a recursive
// allOf) contributes whatever it has so far.
func mergeAllOf(s, m *schema) {
	if m == nil || m == s {
		return
	}
	if s.Type == "" {
		s.Type = m.Type
	}
	if s.Format == "" {
		s.Format = m.Format
	}
	if s.Items == nil {
		s.Items = m.Items
	}
	if s.Example == nil {
		s.Example = m.Example
	}
	if len(s.Enum) == 0 {
		s.Enum = m.Enum
	}
	if len(m.Properties) > 0 && s.Properties == nil {
		s.Properties = make(map[string]*schema, len(m.Properties))
	}
	for name, p := range m.Properties {
		if _, ok := s.Properties[name]; !ok {
			s.Properties[name] = p
		}
	}
	s.Variants = append(s.Variants, m.Variants...)
}
