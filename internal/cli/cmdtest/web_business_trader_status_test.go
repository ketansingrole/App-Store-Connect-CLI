package cmdtest

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/rudrankriyam/App-Store-Connect-CLI/cmd"
)

// The non-trader shape is live-captured (personal values replaced). The trader
// entity is synthetic and uses the status label keys App Store Connect ships.
const traderStatusFixture = `{"accountId":"provider-public","traderDeclaration":"DECLARED_TRADER","sellerInfo":[` +
	`{"legalEntityId":"entity-a","name":"Seller A","isEuCovered":true,"coveredCountry":["FRA","DEU"],"submitted":"2026-11-03T03:40:00.000Z","verificationStatus":"PENDING_VERIFY","verificationStatusLocKey":"PPM.agreements.UI.DSA.compliance.status.actionNeeded"},` +
	`{"legalEntityId":"entity-b","name":"Seller B","isEuCovered":false,"coveredCountry":["USA"],"verificationStatus":"NEW_STATUS","verificationStatusLocKey":"PPM.agreements.UI.DSA.compliance.status.somethingNew"}]}`

func TestWebBusinessTraderStatus(t *testing.T) {
	for _, tc := range []struct {
		name, response, output string
		status                 int
		wantTable              []string
		wantError              string
	}{
		{name: "complete JSON envelope", response: traderStatusFixture, output: "json"},
		{
			name: "status labels in table", response: traderStatusFixture, output: "table",
			// Apple's label wins over the raw status; an unknown label falls back to it.
			wantTable: []string{"DECLARED_TRADER", "entity-a", "Action Needed", "PENDING_VERIFY", "entity-b", "NEW_STATUS"},
		},
		{
			name: "undeclared account without entities", response: `{"accountId":"provider-public","traderDeclaration":"NOT_DECLARED","sellerInfo":[]}`, output: "table",
			wantTable: []string{"NOT_DECLARED"},
		},
		{name: "forbidden", status: http.StatusForbidden, output: "json", wantError: "web business trader-status failed: Apple denied the request (403)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			installBusinessAgreementsSession(t, "provider-public", func(w http.ResponseWriter, req *http.Request) {
				calls++
				if req.Method != http.MethodGet || req.URL.String() != "https://appstoreconnect.apple.com/ppm/v1/accounts/provider-public/sellerInfo" || req.Header.Get("X-Csrf-Itc") != "itc" {
					t.Errorf("unexpected request: %s %s csrf=%q", req.Method, req.URL, req.Header.Get("X-Csrf-Itc"))
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if tc.status != 0 {
					w.WriteHeader(tc.status)
				}
				_, _ = w.Write([]byte(tc.response))
			})
			stdout, stderr, code := runWithDeps(t, nil, "web", "business", "trader-status", "--output", tc.output)
			if tc.wantError != "" {
				if code == cmd.ExitSuccess || stdout != "" || !strings.Contains(stderr, tc.wantError) || calls != 1 {
					t.Fatalf("code=%d stdout=%q stderr=%q calls=%d", code, stdout, stderr, calls)
				}
				return
			}
			if code != cmd.ExitSuccess || stderr != "" || calls != 1 {
				t.Fatalf("code=%d stderr=%q calls=%d", code, stderr, calls)
			}
			if tc.output == "json" {
				var got, want any
				if err := json.Unmarshal([]byte(stdout), &got); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal([]byte(tc.response), &want); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("response envelope changed: %s", stdout)
				}
				return
			}
			for _, want := range tc.wantTable {
				if !strings.Contains(stdout, want) {
					t.Fatalf("table is missing %q: %s", want, stdout)
				}
			}
		})
	}
}
