package openapi

import (
	"fmt"
	"strings"
)

// Reference resolution is local-only by design: a $ref must start with "#/"
// and point inside the same document. External refs (other files, URLs) are
// rejected with an error — Mimic never fetches remote documents.
//
// Supported targets:
//
//	#/components/schemas/Foo
//	#/components/schemas/Foo/properties/bar   (also .../items)
//	#/components/parameters/Foo
//	#/components/requestBodies/Foo
//	#/components/responses/Foo
//
// Resolution happens once at load time and replaces each $ref with its
// target, so the rest of the package never sees a $ref. Schemas are shared by
// pointer rather than copied, which keeps recursive schemas representable (as
// cycles) instead of expanding them into an infinite tree.
//
// The adapter has no OpenAPI library yet (see the note on doc), so this small
// resolver stands in for one; it should be dropped when the library lands.

// resolveRefs rewrites every $ref in d to point at its target definition.
func resolveRefs(d *doc) error {
	r := &resolver{doc: d, done: map[*schema]bool{}}
	c := &d.Components

	for _, name := range sortedKeys(c.Schemas) {
		s := c.Schemas[name]
		if err := r.walkSchema(&s); err != nil {
			return fmt.Errorf("components.schemas.%s: %w", name, err)
		}
		c.Schemas[name] = s
	}
	for _, name := range sortedKeys(c.Parameters) {
		p, err := r.parameter(*c.Parameters[name])
		if err != nil {
			return fmt.Errorf("components.parameters.%s: %w", name, err)
		}
		c.Parameters[name] = &p
	}
	for _, name := range sortedKeys(c.RequestBodies) {
		rb, err := r.requestBody(*c.RequestBodies[name])
		if err != nil {
			return fmt.Errorf("components.requestBodies.%s: %w", name, err)
		}
		c.RequestBodies[name] = &rb
	}
	for _, name := range sortedKeys(c.Responses) {
		resp, err := r.response(*c.Responses[name])
		if err != nil {
			return fmt.Errorf("components.responses.%s: %w", name, err)
		}
		c.Responses[name] = &resp
	}

	for _, path := range sortedKeys(d.Paths) {
		ops := d.Paths[path]
		for _, method := range sortedKeys(ops) {
			op := ops[method]
			if err := r.operation(&op); err != nil {
				return fmt.Errorf("paths.%s.%s: %w", path, method, err)
			}
			ops[method] = op
		}
	}
	return nil
}

type resolver struct {
	doc  *doc
	done map[*schema]bool // schemas whose children are already resolved
}

func (r *resolver) operation(op *operation) error {
	for code, resp := range op.Responses {
		resolved, err := r.response(resp)
		if err != nil {
			return fmt.Errorf("responses.%s: %w", code, err)
		}
		op.Responses[code] = resolved
	}
	if op.RequestBody != nil {
		rb, err := r.requestBody(*op.RequestBody)
		if err != nil {
			return fmt.Errorf("requestBody: %w", err)
		}
		op.RequestBody = &rb
	}
	for i, p := range op.Parameters {
		resolved, err := r.parameter(p)
		if err != nil {
			return fmt.Errorf("parameters[%d]: %w", i, err)
		}
		op.Parameters[i] = resolved
	}
	return nil
}

func (r *resolver) response(resp response) (response, error) {
	resp, err := follow(resp, resp.Ref, "responses", r.doc.Components.Responses, func(x response) string { return x.Ref })
	if err != nil {
		return resp, err
	}
	return resp, r.content(resp.Content)
}

func (r *resolver) requestBody(rb requestBody) (requestBody, error) {
	rb, err := follow(rb, rb.Ref, "requestBodies", r.doc.Components.RequestBodies, func(x requestBody) string { return x.Ref })
	if err != nil {
		return rb, err
	}
	return rb, r.content(rb.Content)
}

func (r *resolver) parameter(p parameter) (parameter, error) {
	p, err := follow(p, p.Ref, "parameters", r.doc.Components.Parameters, func(x parameter) string { return x.Ref })
	if err != nil {
		return p, err
	}
	return p, r.walkSchema(&p.Schema)
}

// follow chases a chain of $refs into components.<kind> until it reaches a
// definition that isn't a ref. A chain that loops back on itself is an error.
func follow[T any](v T, ref, kind string, defs map[string]*T, refOf func(T) string) (T, error) {
	seen := map[string]bool{}
	for ref != "" {
		if seen[ref] {
			return v, fmt.Errorf("circular reference %q", ref)
		}
		seen[ref] = true

		name, err := componentName(ref, kind)
		if err != nil {
			return v, err
		}
		target := defs[name]
		if target == nil {
			return v, fmt.Errorf("unresolved reference %q", ref)
		}
		v = *target
		ref = refOf(v)
	}
	return v, nil
}

func (r *resolver) content(content map[string]mediaType) error {
	for ct, mt := range content {
		if err := r.walkSchema(&mt.Schema); err != nil {
			return fmt.Errorf("content.%s: %w", ct, err)
		}
		content[ct] = mt
	}
	return nil
}

// walkSchema replaces *sp with its fully dereferenced target and then
// resolves the target's children. Each schema's children are visited once
// (r.done), which is what terminates recursion on self-referencing schemas.
func (r *resolver) walkSchema(sp **schema) error {
	s, err := r.deref(*sp)
	if err != nil {
		return err
	}
	*sp = s
	if s == nil || r.done[s] {
		return nil
	}
	r.done[s] = true

	for _, name := range sortedKeys(s.Properties) {
		child := s.Properties[name]
		if err := r.walkSchema(&child); err != nil {
			return fmt.Errorf("properties.%s: %w", name, err)
		}
		s.Properties[name] = child
	}
	if err := r.walkSchema(&s.Items); err != nil {
		return fmt.Errorf("items: %w", err)
	}
	return nil
}

// deref follows a chain of $refs (a ref to a ref) until it reaches a schema
// that isn't one. A chain that loops back on itself is an error: unlike a
// recursive property, it never reaches a concrete schema.
func (r *resolver) deref(s *schema) (*schema, error) {
	seen := map[string]bool{}
	for s != nil && s.Ref != "" {
		if seen[s.Ref] {
			return nil, fmt.Errorf("circular reference %q", s.Ref)
		}
		seen[s.Ref] = true

		target, err := r.lookupSchema(s.Ref)
		if err != nil {
			return nil, err
		}
		s = target
	}
	return s, nil
}

// lookupSchema finds the schema a ref points at, walking any
// /properties/<name> and /items segments after the component name. Parents
// along the way are dereferenced, so a path may pass through another $ref.
func (r *resolver) lookupSchema(ref string) (*schema, error) {
	rest, err := localPointer(ref, "schemas")
	if err != nil {
		return nil, err
	}
	segs := strings.Split(rest, "/")
	cur := r.doc.Components.Schemas[unescapePointer(segs[0])]
	if cur == nil {
		return nil, fmt.Errorf("unresolved reference %q", ref)
	}

	for i := 1; i < len(segs); i++ {
		if cur, err = r.deref(cur); err != nil {
			return nil, err
		}
		if cur == nil {
			break
		}
		switch segs[i] {
		case "items":
			cur = cur.Items
		case "properties":
			i++
			if i >= len(segs) {
				return nil, fmt.Errorf("unresolved reference %q", ref)
			}
			cur = cur.Properties[unescapePointer(segs[i])]
		default:
			return nil, fmt.Errorf("unsupported reference %q: segment %q is not supported (only properties/<name> and items)", ref, segs[i])
		}
		if cur == nil {
			break
		}
	}
	if cur == nil {
		return nil, fmt.Errorf("unresolved reference %q", ref)
	}
	return cur, nil
}

// componentName extracts <name> from "#/components/<kind>/<name>".
func componentName(ref, kind string) (string, error) {
	rest, err := localPointer(ref, kind)
	if err != nil {
		return "", err
	}
	return unescapePointer(rest), nil
}

// localPointer validates that ref is a local pointer into components.<kind>
// and returns the part after it.
func localPointer(ref, kind string) (string, error) {
	if !strings.HasPrefix(ref, "#/") {
		return "", fmt.Errorf("unsupported reference %q: only local references (#/...) are supported, external documents are never fetched", ref)
	}
	prefix := "#/components/" + kind + "/"
	rest, ok := strings.CutPrefix(ref, prefix)
	if !ok || rest == "" {
		return "", fmt.Errorf("unsupported reference %q: expected %s<name>", ref, prefix)
	}
	return rest, nil
}

// unescapePointer decodes RFC 6901 escapes.
func unescapePointer(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~1", "/"), "~0", "~")
}
