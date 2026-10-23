package openapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
)

// ValidationMode controls whether requests that violate a spec's contract
// reach the simulated behavior.
type ValidationMode string

const (
	// ValidationPermissive serves invalid requests anyway, flagging them
	// with an X-Mimic-Validation-Warning header. The default.
	ValidationPermissive ValidationMode = "permissive"
	// ValidationStrict rejects invalid requests with 400 VALIDATION_ERROR
	// before any simulated behavior runs.
	ValidationStrict ValidationMode = "strict"
)

// ParseValidationMode parses MIMIC_OPENAPI_VALIDATION / x-mimic-validation.
func ParseValidationMode(s string) (ValidationMode, error) {
	switch m := ValidationMode(strings.ToLower(strings.TrimSpace(s))); m {
	case ValidationPermissive, ValidationStrict:
		return m, nil
	case "":
		return ValidationPermissive, nil
	default:
		return "", fmt.Errorf("validation mode must be permissive or strict, got %q", s)
	}
}

// validationIssue is one contract violation, in the same
// {"error":{"code":"VALIDATION_ERROR","issues":[...]}} envelope the admin
// API uses. In is path/query/header/cookie/body; Name is the parameter
// name or the dotted body field path (empty for the body as a whole).
type validationIssue struct {
	In     string `json:"in"`
	Name   string `json:"name,omitempty"`
	Reason string `json:"reason"`
}

// requestValidator checks a request against one operation's contract.
type requestValidator func(r *http.Request) []validationIssue

var validationOptions = &openapi3filter.Options{
	MultiError: true,
	// Spec-derived routes don't enforce auth (see provider.go), so
	// security requirements are not part of request validation either.
	AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
}

// newRequestValidator builds the validator for one kin-openapi operation.
// Path parameters come from the ServeMux match (r.PathValue), so no kin
// router is needed.
func newRequestValidator(spec *openapi3.T, path, method string, item *openapi3.PathItem, op *openapi3.Operation, pathParams []string) requestValidator {
	route := &routers.Route{Spec: spec, Path: path, PathItem: item, Method: method, Operation: op}
	return func(r *http.Request) []validationIssue {
		params := make(map[string]string, len(pathParams))
		for _, name := range pathParams {
			params[name] = r.PathValue(name)
		}

		// kin-openapi consumes the body; buffer it so the handler can
		// still read it afterwards.
		var body []byte
		if r.Body != nil {
			body, _ = io.ReadAll(r.Body)
			_ = r.Body.Close()
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		err := openapi3filter.ValidateRequest(context.Background(), &openapi3filter.RequestValidationInput{
			Request:    r,
			PathParams: params,
			Route:      route,
			Options:    validationOptions,
		})
		r.Body = io.NopCloser(bytes.NewReader(body))
		if err == nil {
			return nil
		}
		return toIssues(err, validationIssue{})
	}
}

// toIssues flattens kin-openapi's (possibly nested multi-) errors into
// validationIssues, inheriting In/Name from the enclosing error.
func toIssues(err error, base validationIssue) []validationIssue {
	var multi openapi3.MultiError
	if errors.As(err, &multi) {
		var out []validationIssue
		for _, e := range multi {
			out = append(out, toIssues(e, base)...)
		}
		return out
	}

	var reqErr *openapi3filter.RequestError
	if errors.As(err, &reqErr) {
		switch {
		case reqErr.Parameter != nil:
			base.In, base.Name = reqErr.Parameter.In, reqErr.Parameter.Name
		case reqErr.RequestBody != nil:
			base.In = "body"
		}
		if reqErr.Err != nil {
			return toIssues(reqErr.Err, withReason(base, reqErr.Reason))
		}
		return []validationIssue{withReason(base, reqErr.Reason)}
	}

	var schemaErr *openapi3.SchemaError
	if errors.As(err, &schemaErr) {
		// Parameter schema errors always arrive wrapped in a RequestError
		// naming the parameter; a bare SchemaError (kin-openapi's
		// MultiError mode returns body errors unwrapped) is the body's.
		if base.In == "" {
			base.In = "body"
		}
		if ptr := schemaErr.JSONPointer(); len(ptr) > 0 && base.In == "body" {
			base.Name = strings.Join(ptr, ".")
		}
		base.Reason = schemaErr.Reason
		return []validationIssue{base}
	}

	var parseErr *openapi3filter.ParseError
	if errors.As(err, &parseErr) {
		base.Reason = parseErr.Error()
		return []validationIssue{base}
	}

	if base.Reason == "" {
		base.Reason = err.Error()
	}
	return []validationIssue{base}
}

func withReason(i validationIssue, reason string) validationIssue {
	if i.Reason == "" {
		i.Reason = reason
	}
	return i
}

// withValidation wraps next so requests are checked against the route's
// contract first. Strict mode rejects violations; permissive mode serves
// them with a warning header.
func withValidation(route Route, next http.Handler) http.Handler {
	if route.validate == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issues := route.validate(r)
		if len(issues) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		if route.Validation == ValidationStrict {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": map[string]any{"code": "VALIDATION_ERROR", "issues": issues},
			})
			return
		}
		first := issues[0]
		w.Header().Set("X-Mimic-Validation-Warning", fmt.Sprintf("%d contract violation(s); first: %s %s: %s", len(issues), first.In, first.Name, first.Reason))
		next.ServeHTTP(w, r)
	})
}
