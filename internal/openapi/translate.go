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
	if _, err := decodeExtension(k.Extensions, "x-mimic-name", &d.MimicName); err != nil {
		return doc{}, err
	}
	var persist bool
	ok, err := decodeExtension(k.Extensions, "x-mimic-persist", &persist)
	if err != nil {
		return doc{}, err
	}
	if ok {
		d.MimicPersist = &persist
	}
	var validation string
	ok, err = decodeExtension(k.Extensions, "x-mimic-validation", &validation)
	if err != nil {
		return doc{}, err
	}
	if ok {
		mode, err := ParseValidationMode(validation)
		if err != nil {
			return doc{}, fmt.Errorf("x-mimic-validation: %w", err)
		}
		d.MimicValidation = &mode
	}

	if k.Paths == nil {
		return d, nil
	}
	schemas := newSchemaTranslator()
	for path, item := range k.Paths.Map() {
		if item == nil {
			continue
		}
		ops := map[string]operation{}
		for method, kop := range item.Operations() {
			op, err := fromKinOperation(kop, item.Parameters, schemas)
			if err != nil {
				return doc{}, fmt.Errorf("%s %s: %w", strings.ToUpper(method), path, err)
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
		d.Paths[path] = ops
	}
	return d, nil
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

	// Path-item-level parameters apply to every operation unless the
	// operation redeclares the same name+location.
	seen := map[string]bool{}
	for _, list := range []openapi3.Parameters{k.Parameters, pathParams} {
		for _, ref := range list {
			if ref == nil || ref.Value == nil {
				continue
			}
			p := ref.Value
			key := p.In + ":" + p.Name
			if seen[key] {
				continue
			}
			seen[key] = true
			op.Parameters = append(op.Parameters, parameter{
				Name:     p.Name,
				In:       p.In,
				Required: p.Required,
				Schema:   schemas.translate(p.Schema),
			})
		}
	}

	if k.RequestBody != nil && k.RequestBody.Value != nil {
		op.RequestBody = &requestBody{
			Required: k.RequestBody.Value.Required,
			Content:  fromKinContent(k.RequestBody.Value.Content, schemas),
		}
	}

	if k.Responses != nil {
		for code, ref := range k.Responses.Map() {
			if ref == nil || ref.Value == nil {
				continue
			}
			op.Responses[code] = response{Content: fromKinContent(ref.Value.Content, schemas)}
		}
	}
	return op, nil
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
	s := &schema{Format: k.Format, Example: k.Example, Enum: k.Enum}
	t.done[k] = s
	if k.Type != nil {
		for _, typ := range k.Type.Slice() {
			if typ != "null" {
				s.Type = typ
				break
			}
		}
	}
	if len(k.Properties) > 0 {
		s.Properties = make(map[string]*schema, len(k.Properties))
		for name, p := range k.Properties {
			s.Properties[name] = t.translate(p)
		}
	}
	s.Items = t.translate(k.Items)

	for _, ref := range k.AllOf {
		mergeAllOf(s, t.translate(ref))
	}
	for _, list := range []openapi3.SchemaRefs{k.OneOf, k.AnyOf} {
		for _, ref := range list {
			if v := t.translate(ref); v != nil {
				s.Variants = append(s.Variants, v)
			}
		}
	}
	if s.Type == "" && len(s.Properties) > 0 {
		s.Type = "object"
	}
	return s
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
