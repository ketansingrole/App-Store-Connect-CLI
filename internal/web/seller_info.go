package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// EU Digital Services Act trader declarations, as App Store Connect Business
// stores them for the provider account.
const (
	TraderDeclarationNotDeclared = "NOT_DECLARED"
	TraderDeclarationTrader      = "DECLARED_TRADER"
	TraderDeclarationNotTrader   = "DECLARED_NOT_TRADER"
)

// TraderStatus is the status label App Store Connect Business shows for a
// legal entity's trader information.
type TraderStatus string

// Apple picks one of these labels for each legal entity through
// verificationStatusLocKey; the raw verificationStatus does not map to them
// one to one (a non-trader reports NOT_PROVIDED and shows Active).
const (
	TraderStatusActive       TraderStatus = "Active"
	TraderStatusVerifying    TraderStatus = "Verifying"
	TraderStatusInReview     TraderStatus = "In Review"
	TraderStatusActionNeeded TraderStatus = "Action Needed"
	TraderStatusRejected     TraderStatus = "Rejected"
	TraderStatusNotDeclared  TraderStatus = "Not Declared"
	TraderStatusUnknown      TraderStatus = "Unknown"
)

const traderStatusLocKeyPrefix = "PPM.agreements.UI.DSA.compliance.status."

var traderStatusByLocKey = map[string]TraderStatus{
	"active":       TraderStatusActive,
	"verifying":    TraderStatusVerifying,
	"inReview":     TraderStatusInReview,
	"actionNeeded": TraderStatusActionNeeded,
	"rejected":     TraderStatusRejected,
	"notDeclared":  TraderStatusNotDeclared,
	"unknown":      TraderStatusUnknown,
}

// SellerInfo is the account's EU Digital Services Act trader information.
// Raw keeps Apple's complete response for JSON output.
type SellerInfo struct {
	Raw               json.RawMessage
	AccountID         string
	TraderDeclaration string
	LegalEntities     []SellerLegalEntity
}

// SellerLegalEntity is the trader information of one legal entity.
type SellerLegalEntity struct {
	LegalEntityID            string   `json:"legalEntityId"`
	Name                     string   `json:"name"`
	IsEUCovered              bool     `json:"isEuCovered"`
	CoveredCountries         []string `json:"coveredCountry"`
	Submitted                string   `json:"submitted"`
	VerificationStatus       string   `json:"verificationStatus"`
	VerificationStatusLocKey string   `json:"verificationStatusLocKey"`
}

// Status returns the label App Store Connect shows for the entity, or "" when
// Apple sent a label this client does not recognize.
func (e SellerLegalEntity) Status() TraderStatus {
	key, ok := strings.CutPrefix(strings.TrimSpace(e.VerificationStatusLocKey), traderStatusLocKeyPrefix)
	if !ok {
		return ""
	}
	return traderStatusByLocKey[key]
}

// GetSellerInfo reads the selected provider account's EU Digital Services Act
// trader declaration and per-legal-entity verification status, as shown in
// App Store Connect Business. It only reads; declaring trader status requires
// identity verification in App Store Connect.
func (c *Client) GetSellerInfo(ctx context.Context) (*SellerInfo, error) {
	if c == nil || c.httpClient == nil {
		return nil, errors.New("seller info web client is not configured")
	}
	accountID := strings.TrimSpace(c.publicProviderID)
	if accountID == "" {
		return nil, errors.New("seller info requires a selected public provider ID")
	}
	baseURL := c.complianceFormBaseURL()
	headers := make(http.Header)
	headers.Set("X-Requested-With", "XMLHttpRequest")
	headers.Set("X-Csrf-Itc", "itc")
	headers.Set("Origin", baseURL)
	headers.Set("Referer", strings.TrimRight(baseURL, "/")+"/business")
	path := "/ppm/v1/accounts/" + url.PathEscape(accountID) + "/sellerInfo"
	body, err := c.doRequestBase(ctx, baseURL, http.MethodGet, path, nil, headers)
	if err != nil {
		return nil, err
	}
	var response struct {
		AccountID         string              `json:"accountId"`
		TraderDeclaration string              `json:"traderDeclaration"`
		SellerInfo        []SellerLegalEntity `json:"sellerInfo"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, errors.New("seller info returned a malformed response")
	}
	return &SellerInfo{
		Raw:               body,
		AccountID:         response.AccountID,
		TraderDeclaration: strings.TrimSpace(response.TraderDeclaration),
		LegalEntities:     response.SellerInfo,
	}, nil
}
