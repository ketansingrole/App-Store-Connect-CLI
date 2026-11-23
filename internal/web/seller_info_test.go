package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sellerInfoBody mirrors the live response shape for a non-trader account,
// with personal values replaced.
const sellerInfoBody = `{
	"accountId":"provider-123",
	"traderDeclaration":"DECLARED_NOT_TRADER",
	"sellerInfo":[{
		"legalEntityId":"entity-1",
		"name":"Example Seller",
		"sapVendorNumber":"12345678",
		"isEuCovered":true,
		"coveredCountry":["FRA","DEU","USA"],
		"submitted":"2026-11-03T03:40:00.000Z",
		"verificationStatus":"NOT_PROVIDED",
		"verificationStatusLocKey":"PPM.agreements.UI.DSA.compliance.status.active"
	}]
}`

func TestGetSellerInfoReadsTraderStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/ppm/v1/accounts/provider-123/sellerInfo" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if got := r.Header.Get("X-Csrf-Itc"); got != "itc" {
			t.Fatalf("expected X-Csrf-Itc itc, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(sellerInfoBody))
	}))
	defer server.Close()

	client := testWebClient(server)
	client.publicProviderID = "provider-123"
	got, err := client.GetSellerInfo(context.Background())
	if err != nil {
		t.Fatalf("GetSellerInfo() error = %v", err)
	}
	if got.AccountID != "provider-123" || got.TraderDeclaration != TraderDeclarationNotTrader {
		t.Fatalf("unexpected account fields: %#v", got)
	}
	if len(got.LegalEntities) != 1 {
		t.Fatalf("expected one legal entity, got %#v", got.LegalEntities)
	}
	entity := got.LegalEntities[0]
	if entity.LegalEntityID != "entity-1" || !entity.IsEUCovered || entity.VerificationStatus != "NOT_PROVIDED" ||
		len(entity.CoveredCountries) != 3 || entity.Submitted != "2026-11-03T03:40:00.000Z" {
		t.Fatalf("unexpected legal entity: %#v", entity)
	}
	if entity.Status() != TraderStatusActive {
		t.Fatalf("Status() = %q, want %q", entity.Status(), TraderStatusActive)
	}
	if string(got.Raw) != sellerInfoBody {
		t.Fatalf("expected Apple's response to be preserved, got %s", got.Raw)
	}
}

func TestSellerLegalEntityStatusUsesAppleLabel(t *testing.T) {
	tests := []struct {
		locKey string
		want   TraderStatus
	}{
		{"PPM.agreements.UI.DSA.compliance.status.active", TraderStatusActive},
		{"PPM.agreements.UI.DSA.compliance.status.verifying", TraderStatusVerifying},
		{"PPM.agreements.UI.DSA.compliance.status.inReview", TraderStatusInReview},
		{"PPM.agreements.UI.DSA.compliance.status.actionNeeded", TraderStatusActionNeeded},
		{"PPM.agreements.UI.DSA.compliance.status.rejected", TraderStatusRejected},
		{"PPM.agreements.UI.DSA.compliance.status.notDeclared", TraderStatusNotDeclared},
		{"PPM.agreements.UI.DSA.compliance.status.unknown", TraderStatusUnknown},
		{"PPM.agreements.UI.DSA.compliance.status.somethingNew", ""},
		{"", ""},
	}
	for _, test := range tests {
		entity := SellerLegalEntity{VerificationStatus: "NOT_PROVIDED", VerificationStatusLocKey: test.locKey}
		if got := entity.Status(); got != test.want {
			t.Errorf("Status() for %q = %q, want %q", test.locKey, got, test.want)
		}
	}
}

func TestGetSellerInfoErrors(t *testing.T) {
	tests := []struct {
		name       string
		providerID string
		status     int
		body       string
		wantErr    string
	}{
		{name: "no provider", providerID: " ", wantErr: "public provider ID"},
		{name: "forbidden", providerID: "provider-123", status: http.StatusForbidden, body: ``, wantErr: "403"},
		{name: "malformed", providerID: "provider-123", status: http.StatusOK, body: `[]`, wantErr: "malformed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()

			client := testWebClient(server)
			client.publicProviderID = test.providerID
			_, err := client.GetSellerInfo(context.Background())
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("GetSellerInfo() error = %v, want %q", err, test.wantErr)
			}
			if test.status == 0 && requests != 0 {
				t.Fatalf("expected no request without a provider, got %d", requests)
			}
		})
	}
}
