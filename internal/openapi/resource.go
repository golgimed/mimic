package openapi

import "strings"

// ResourceOp is the CRUD operation a route maps to when persistence is
// enabled, inferred purely from REST path shape + HTTP method — no spec
// annotation needed for the common case.
type ResourceOp int

const (
	// OpNone means this route isn't CRUD-shaped (or persistence is off for
	// its spec): it keeps today's static example/schema-stub behavior.
	OpNone ResourceOp = iota
	OpList
	OpCreate
	OpRead
	OpUpdate
	OpDelete
)

// inferResource maps an un-prefixed spec path (e.g. "/pets/{id}") to a
// resource type and id path-parameter name.
//
// v1 only understands single-level resources: a collection path with no
// path parameters, or an item path with exactly one, trailing path
// parameter. Anything else (nested resources like "/pets/{id}/vaccinations",
// multiple path parameters, etc.) is reported unsupported and falls back to
// static behavior — see specs/README.md.
func inferResource(path string) (resourceType, idParam string, isItem, supported bool) {
	segs := strings.Split(strings.Trim(path, "/"), "/")
	if len(segs) == 0 || segs[0] == "" {
		return "", "", false, false
	}

	paramCount := 0
	for _, s := range segs {
		if isPathParam(s) {
			paramCount++
		}
	}

	switch paramCount {
	case 0:
		return segs[0], "", false, true
	case 1:
		last := segs[len(segs)-1]
		if !isPathParam(last) {
			return "", "", false, false
		}
		return segs[0], strings.Trim(last, "{}"), true, true
	default:
		return "", "", false, false
	}
}

func isPathParam(seg string) bool {
	return strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}")
}

// inferOp maps an HTTP method + resource shape to a CRUD operation, plain
// REST convention.
func inferOp(method string, isItem bool) ResourceOp {
	switch {
	case !isItem && method == "GET":
		return OpList
	case !isItem && method == "POST":
		return OpCreate
	case isItem && method == "GET":
		return OpRead
	case isItem && (method == "PUT" || method == "PATCH"):
		return OpUpdate
	case isItem && method == "DELETE":
		return OpDelete
	default:
		return OpNone
	}
}
