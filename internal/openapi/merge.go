package openapi

import "fmt"

// ConflictMode controls what happens when two specs declare the same
// method+path.
type ConflictMode string

const (
	// ConflictStrict fails startup on any duplicate route.
	ConflictStrict ConflictMode = "strict"
	// ConflictMerge lets the last spec (by discovery/CLI order) win.
	ConflictMerge ConflictMode = "merge"
	// ConflictPriority is merge with explicit ordering: specs are already
	// sorted by priority (first = highest) before calling Merge, so this
	// mode keeps the *first* spec's route on conflict instead of the last.
	ConflictPriority ConflictMode = "priority"
)

// Merge combines routes from multiple specs into one route table according
// to mode. specRoutes must be in discovery/priority order.
func Merge(specRoutes [][]Route, mode ConflictMode) ([]Route, error) {
	byKey := make(map[string]Route)
	order := make([]string, 0)

	for _, routes := range specRoutes {
		for _, route := range routes {
			key := route.Method + " " + route.Path
			existing, seen := byKey[key]

			switch {
			case !seen:
				byKey[key] = route
				order = append(order, key)

			case mode == ConflictStrict:
				return nil, fmt.Errorf("conflicting route %s: declared in both %q and %q", key, existing.Source, route.Source)

			case mode == ConflictMerge:
				byKey[key] = route // last wins

			case mode == ConflictPriority:
				// first wins: keep existing, ignore route

			default:
				return nil, fmt.Errorf("unknown conflict mode %q", mode)
			}
		}
	}

	out := make([]Route, 0, len(order))
	for _, key := range order {
		out = append(out, byKey[key])
	}
	return out, nil
}
