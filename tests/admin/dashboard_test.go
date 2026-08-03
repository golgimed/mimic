package admin_test

import (
	"net/url"
	"sort"
	"strings"
	"testing"

	"github.com/golgimed/mimic/internal/testutil"
)

func TestListsItemsAcrossProviders(t *testing.T) {
	app := testutil.New(t, 60000000000) // 60s, matches ZENVIA_STATUS_DELAY_MS=60000 in the original test

	zenviaRec := app.Do(t, "POST", "/zenvia/channels/sms/messages", map[string]string{"X-API-TOKEN": "test-token"}, map[string]any{
		"from": "sms-account", "to": "5510888", "contents": []map[string]any{{"type": "text", "text": "hi"}},
	})
	var zenviaBody struct {
		ID string `json:"id"`
	}
	testutil.DecodeJSON(t, zenviaRec, &zenviaBody)

	q := url.Values{"secret_data": {"abc"}, "callback_uri": {"https://my.app/cb"}, "autostart": {"true"}}
	authRec := app.Do(t, "GET", "/integraicp/c/test-channel/icp/v3/authentications?"+q.Encode(), nil, nil)
	loc, err := url.Parse(authRec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	credentialID := loc.Query().Get("credentialId")

	listRec := app.Do(t, "GET", "/admin/items", nil, nil)
	if listRec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", listRec.Code, listRec.Body.String())
	}
	var list struct {
		Content []map[string]any `json:"content"`
	}
	testutil.DecodeJSON(t, listRec, &list)
	if len(list.Content) != 2 {
		t.Fatalf("expected 2 items, got %d", len(list.Content))
	}
	var providers []string
	for _, item := range list.Content {
		providers = append(providers, item["provider"].(string))
	}
	sort.Strings(providers)
	if strings.Join(providers, ",") != "integraicp,zenvia" {
		t.Errorf("providers = %v", providers)
	}

	zenviaDetail := app.Do(t, "GET", "/admin/items/zenvia/"+zenviaBody.ID, nil, nil)
	if zenviaDetail.Code != 200 {
		t.Fatalf("expected 200, got %d", zenviaDetail.Code)
	}
	var zenviaDetailBody struct {
		Payload struct {
			ID string `json:"id"`
		} `json:"payload"`
	}
	testutil.DecodeJSON(t, zenviaDetail, &zenviaDetailBody)
	if zenviaDetailBody.Payload.ID != zenviaBody.ID {
		t.Errorf("payload.id = %v, want %v", zenviaDetailBody.Payload.ID, zenviaBody.ID)
	}

	icpDetail := app.Do(t, "GET", "/admin/items/integraicp/"+credentialID, nil, nil)
	if icpDetail.Code != 200 {
		t.Fatalf("expected 200, got %d", icpDetail.Code)
	}
	var icpDetailBody struct {
		Payload struct {
			ID string `json:"id"`
		} `json:"payload"`
	}
	testutil.DecodeJSON(t, icpDetail, &icpDetailBody)
	if icpDetailBody.Payload.ID != credentialID {
		t.Errorf("payload.id = %v, want %v", icpDetailBody.Payload.ID, credentialID)
	}
}

func TestUnknownItem404(t *testing.T) {
	app := testutil.New(t, 0)
	rec := app.Do(t, "GET", "/admin/items/zenvia/does-not-exist", nil, nil)
	if rec.Code != 404 {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestServesDashboardHTML(t *testing.T) {
	app := testutil.New(t, 0)
	rec := app.Do(t, "GET", "/dashboard", nil, nil)
	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Errorf("content-type = %v", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "Mimic") {
		t.Errorf("expected body to contain Mimic")
	}
}
