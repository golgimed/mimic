package openapi

import (
	"sort"
	"strconv"
	"strings"
)

const contentTypeJSON = "application/json"

// Route is this package's private internal model: everything needed to
// register and serve one operation from a spec. It never leaves this
// package — provider.go converts a []Route into a registry.Provider.
type Route struct {
	Method string
	Path   string
	Plan   ResponsePlan
	Source string // spec file path, for diagnostics/dashboard

	// SpecName scopes persisted resources to the spec that declared them
	// (its route prefix — unique per loaded spec).
	SpecName string

	// Op, ResourceType and IDParam describe this route's inferred REST
	// shape (see resource.go); Op is OpNone for non-CRUD-shaped routes.
	// Persist is the effective persistence setting for this route's spec —
	// only consulted when Op != OpNone.
	Op           ResourceOp
	ResourceType string
	IDParam      string
	Persist      bool

	// Behavior is this route's x-mimic-behavior declaration, if any (see
	// behavior.go). Seeded into fault_config at boot by providers.go.
	Behavior *behaviorSpec
}

// ResponsePlan describes how to build the mocked response body for a route.
type ResponsePlan struct {
	StatusCode  int
	ContentType string
	Example     any     // used verbatim if present
	Schema      *schema // used to generate Generated if Example is nil
	Generated   any     // schema-derived body, precomputed once at boot (see seededFaker)
}

// BuildRoutes converts a parsed spec into routes, prefixing every path with
// "/"+prefix so spec-derived providers don't collide with hand-written ones
// or with each other (mirrors how existing providers use Name as their mux
// prefix). persist is the effective persistence setting for this spec
// (spec.Persist override, or the global MIMIC_OPENAPI_PERSIST default),
// applied to every CRUD-shaped route.
func BuildRoutes(spec *LoadedSpec, prefix string, persist bool) []Route {
	var routes []Route

	for _, path := range sortedKeys(spec.doc.Paths) {
		resourceType, idParam, isItem, supported := inferResource(path)
		methods := spec.doc.Paths[path]

		for _, m := range sortedKeys(methods) {
			if !httpMethods[strings.ToLower(m)] {
				continue
			}
			routes = append(routes, buildRoute(spec, prefix, path, m, methods[m], resourceType, idParam, isItem, supported, persist))
		}
	}

	return routes
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// buildRoute converts one spec operation into a Route, filling in the
// inferred REST shape (resourceType/idParam/Op/Persist) only when supported
// is true — non-CRUD-shaped routes are served straight from Plan.
func buildRoute(spec *LoadedSpec, prefix, path, m string, op operation, resourceType, idParam string, isItem, supported, persist bool) Route {
	method := strings.ToUpper(m)
	routePath := "/" + strings.Trim(prefix, "/") + path

	plan := buildResponsePlan(op)
	if plan.Example == nil && plan.Schema != nil {
		plan.Generated = generateFromSchema(plan.Schema, seededFaker(method+" "+routePath))
	}

	route := Route{
		Method:   method,
		Path:     routePath,
		Plan:     plan,
		Source:   spec.Path,
		SpecName: prefix,
		Behavior: op.Behavior,
	}
	if supported {
		route.ResourceType = resourceType
		route.IDParam = idParam
		route.Op = inferOp(method, isItem)
		route.Persist = persist && route.Op != OpNone
	}
	return route
}

func buildResponsePlan(op operation) ResponsePlan {
	status, resp, ok := pickResponse(op.Responses)
	if !ok {
		return ResponsePlan{StatusCode: 200, ContentType: contentTypeJSON}
	}

	mt, contentType, ok := pickMediaType(resp.Content)
	if !ok {
		return ResponsePlan{StatusCode: status, ContentType: contentTypeJSON}
	}

	example := pickExample(mt)
	return ResponsePlan{
		StatusCode:  status,
		ContentType: contentType,
		Example:     example,
		Schema:      mt.Schema,
	}
}

// pickResponse prefers the lowest 2xx status code declared, falling back to
// "default" (reported as 200) if no explicit 2xx exists.
func pickResponse(responses map[string]response) (int, response, bool) {
	best := -1
	var bestResp response
	var def response
	hasDefault := false

	for code, r := range responses {
		if code == "default" {
			def = r
			hasDefault = true
			continue
		}
		n, err := strconv.Atoi(code)
		if err != nil || n < 200 || n >= 300 {
			continue
		}
		if best == -1 || n < best {
			best = n
			bestResp = r
		}
	}

	if best != -1 {
		return best, bestResp, true
	}
	if hasDefault {
		return 200, def, true
	}
	return 0, response{}, false
}

// pickMediaType prefers application/json, falling back to the first
// declared media type (in a deterministic, sorted order).
func pickMediaType(content map[string]mediaType) (mediaType, string, bool) {
	if mt, ok := content[contentTypeJSON]; ok {
		return mt, contentTypeJSON, true
	}
	if len(content) == 0 {
		return mediaType{}, "", false
	}
	types := make([]string, 0, len(content))
	for t := range content {
		types = append(types, t)
	}
	sort.Strings(types)
	return content[types[0]], types[0], true
}

func pickExample(mt mediaType) any {
	if mt.Example != nil {
		return mt.Example
	}
	if len(mt.Examples) > 0 {
		names := make([]string, 0, len(mt.Examples))
		for n := range mt.Examples {
			names = append(names, n)
		}
		sort.Strings(names)
		return mt.Examples[names[0]].Value
	}
	if mt.Schema != nil && mt.Schema.Example != nil {
		return mt.Schema.Example
	}
	return nil
}
