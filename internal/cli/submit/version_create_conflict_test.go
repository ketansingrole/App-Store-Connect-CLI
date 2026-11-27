package submit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/asc"
	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/cli/shared"
)

func TestExplainVersionCreateConflictNamesTheVersionInProgress(t *testing.T) {
	refused := &asc.APIError{StatusCode: http.StatusConflict, Code: "ENTITY_ERROR.RELATIONSHIP.INVALID", Title: "The provided entity includes a relationship with an invalid value", Detail: "You cannot create a new version of the App in the current state."}
	for _, test := range []struct {
		name       string
		createErr  error
		versions   string
		unresolved string
		// wantVersion is the version the error names; empty when the error is returned as it is.
		wantVersion string
		wantHint    []string
		avoidHint   []string
		wantReads   int
	}{
		{
			// 2026-11-27: 2026.11.3 was rejected, its submission still open for the developer's
			// answer, and staging 2026.11.10 was refused without saying why.
			name:        "rejected version with its review unresolved",
			createErr:   refused,
			versions:    `[{"type":"appStoreVersions","id":"version-3","attributes":{"platform":"MAC_OS","versionString":"2026.11.3","appVersionState":"REJECTED","appStoreState":"REJECTED"}},{"type":"appStoreVersions","id":"version-2","attributes":{"platform":"MAC_OS","versionString":"2026.11.2","appVersionState":"READY_FOR_DISTRIBUTION","appStoreState":"READY_FOR_SALE"}}]`,
			unresolved:  `[{"type":"reviewSubmissions","id":"submission-3","attributes":{"state":"UNRESOLVED_ISSUES","platform":"MAC_OS"},"relationships":{"appStoreVersionForReview":{"data":{"type":"appStoreVersions","id":"version-3"}}}}]`,
			wantVersion: "2026.11.3 (REJECTED)",
			wantHint:    []string{`asc review submissions-submit --id "submission-3" --confirm`, `asc versions update --version-id "version-3" --version "2026.11.10"`},
			wantReads:   3,
		},
		{
			name:        "version waiting for review",
			createErr:   refused,
			versions:    `[{"type":"appStoreVersions","id":"version-3","attributes":{"platform":"MAC_OS","versionString":"2026.11.3","appVersionState":"WAITING_FOR_REVIEW","appStoreState":"WAITING_FOR_REVIEW"}}]`,
			unresolved:  `[]`,
			wantVersion: "2026.11.3 (WAITING_FOR_REVIEW)",
			wantHint:    []string{`asc review status --app "app-1"`},
			avoidHint:   []string{"asc versions update", "submissions-submit"},
			wantReads:   2,
		},
		{
			name:      "nothing in progress",
			createErr: refused,
			versions:  `[{"type":"appStoreVersions","id":"version-2","attributes":{"platform":"MAC_OS","versionString":"2026.11.2","appVersionState":"READY_FOR_DISTRIBUTION","appStoreState":"READY_FOR_SALE"}}]`,
			wantReads: 1,
		},
		{
			name:      "not a conflict",
			createErr: &asc.APIError{StatusCode: http.StatusInternalServerError, Code: "UNEXPECTED_ERROR"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			reads := 0
			client := newSubmitTestClient(t, submitRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet {
					return nil, fmt.Errorf("unexpected write: %s %s", req.Method, req.URL.RequestURI())
				}
				reads++
				switch req.URL.Path {
				case "/v1/apps/app-1/appStoreVersions":
					if req.URL.Query().Get("filter[platform]") != "MAC_OS" {
						return nil, fmt.Errorf("versions read lost its platform: %s", req.URL.RequestURI())
					}
					return submitJSONResponse(http.StatusOK, `{"data":`+test.versions+`,"links":{"self":"/v1/apps/app-1/appStoreVersions"}}`)
				case "/v1/apps/app-1/reviewSubmissions":
					return submitJSONResponse(http.StatusOK, `{"data":`+test.unresolved+`,"links":{"self":"/v1/apps/app-1/reviewSubmissions"}}`)
				case "/v1/reviewSubmissions/submission-3/items":
					return submitJSONResponse(http.StatusOK, `{"data":[{"type":"reviewSubmissionItems","id":"item-1","attributes":{"state":"REJECTED"},"relationships":{"appStoreVersion":{"data":{"type":"appStoreVersions","id":"version-3"}}}}],"links":{"self":"/v1/reviewSubmissions/submission-3/items"}}`)
				default:
					return nil, fmt.Errorf("unexpected request: %s %s", req.Method, req.URL.RequestURI())
				}
			}))

			err := ExplainVersionCreateConflict(context.Background(), client, "app-1", "MAC_OS", "2026.11.10", test.createErr)
			if reads != test.wantReads {
				t.Fatalf("reads = %d, want %d", reads, test.wantReads)
			}
			if !errors.Is(err, test.createErr) {
				t.Fatalf("error no longer wraps App Store Connect's: %v", err)
			}
			hinted, ok := errors.AsType[*shared.HintedError](err)
			if test.wantVersion == "" {
				if ok || err.Error() != test.createErr.Error() {
					t.Fatalf("want the error as it was, got %v", err)
				}
				return
			}
			if !ok || !strings.Contains(err.Error(), test.wantVersion) || !strings.Contains(err.Error(), "You cannot create a new version") {
				t.Fatalf("want App Store Connect's refusal naming %s, got %v", test.wantVersion, err)
			}
			for _, want := range test.wantHint {
				if !strings.Contains(hinted.Hint, want) {
					t.Errorf("hint lacks %s: %s", want, hinted.Hint)
				}
			}
			for _, avoid := range test.avoidHint {
				if strings.Contains(hinted.Hint, avoid) {
					t.Errorf("hint suggests %s: %s", avoid, hinted.Hint)
				}
			}
		})
	}
}
