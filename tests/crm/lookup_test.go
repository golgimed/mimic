package crm_test

import (
	"testing"

	"github.com/golgimed/mimic/internal/testutil"
)

// Path shape matches platform-services HTTPCRMClient: the license is
// appended unescaped, so a space goes on the wire as %20.
const path = "/crm/busca-medico-by-name/"

func TestAllowedLicenseReturns200(t *testing.T) {
	app := testutil.New(t, 0)
	rec := app.Do(t, "GET", path+"CRM-SP%2098765", nil, nil)
	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUnknownLicenseReturnsNon200(t *testing.T) {
	app := testutil.New(t, 0)
	for _, lic := range []string{"CRM-SP%20000000", "CRM-RJ%2098765", "98765"} {
		if rec := app.Do(t, "GET", path+lic, nil, nil); rec.Code != 404 {
			t.Errorf("%s: expected 404, got %d", lic, rec.Code)
		}
	}
}
