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
	return s
}
