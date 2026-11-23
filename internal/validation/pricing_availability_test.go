package validation

import (
	"strings"
	"testing"
)

func TestPricingChecks_MissingSchedule(t *testing.T) {
	checks := pricingChecks("app-1", "", false, "", "")
	if !hasCheckID(checks, "pricing.schedule.missing") {
		t.Fatalf("expected pricing.schedule.missing check, got %v", checks)
	}
}

func TestPricingChecks_Pass(t *testing.T) {
	checks := pricingChecks("app-1", "sched-1", false, "USA", "")
	if len(checks) != 0 {
		t.Fatalf("expected no checks, got %d (%v)", len(checks), checks)
	}
}

func TestPricingChecks_MissingBaseTerritoryPrice(t *testing.T) {
	checks := pricingChecks("app-1", "app-1", true, "usa", "")
	if len(checks) != 1 {
		t.Fatalf("expected one check, got %d (%v)", len(checks), checks)
	}
	check := checks[0]
	if check.ID != "pricing.base_price.missing" || check.Severity != SeverityError {
		t.Fatalf("expected blocking pricing.base_price.missing, got %+v", check)
	}
	if check.Message != "app has no price set for base territory USA" {
		t.Fatalf("unexpected message %q", check.Message)
	}
	wantRemediation := `Set a price (Free counts): asc pricing schedule create --app app-1 --free --base-territory "USA"`
	if check.Remediation != wantRemediation {
		t.Fatalf("remediation = %q, want %q", check.Remediation, wantRemediation)
	}
}

func TestPricingChecks_UnverifiedSuppressesMissingBaseTerritoryPrice(t *testing.T) {
	checks := pricingChecks("app-1", "app-1", true, "USA", "pricing access unavailable")
	if hasCheckID(checks, "pricing.base_price.missing") || !hasCheckID(checks, "pricing.schedule.unverified") {
		t.Fatalf("expected only pricing.schedule.unverified, got %v", checks)
	}
}

func TestPricingChecks_UnverifiedWhenSkipped(t *testing.T) {
	checks := pricingChecks("app-1", "", false, "", "pricing access unavailable")
	if !hasCheckID(checks, "pricing.schedule.unverified") {
		t.Fatalf("expected pricing.schedule.unverified check, got %v", checks)
	}
	if hasCheckID(checks, "pricing.schedule.missing") {
		t.Fatalf("did not expect pricing.schedule.missing when pricing is unverified, got %v", checks)
	}
	if checks[0].Severity != SeverityWarning {
		t.Fatalf("expected warning severity, got %s", checks[0].Severity)
	}
}

func TestAvailabilityChecks_MissingAvailability(t *testing.T) {
	checks := availabilityChecks("app-1", "", 0, "")
	if !hasCheckID(checks, "availability.missing") {
		t.Fatalf("expected availability.missing check, got %v", checks)
	}
}

func TestAvailabilityChecks_NoTerritories(t *testing.T) {
	checks := availabilityChecks("app-1", "avail-1", 0, "")
	if !hasCheckID(checks, "availability.territories.none") {
		t.Fatalf("expected availability.territories.none check, got %v", checks)
	}
}

func TestAvailabilityChecks_Pass(t *testing.T) {
	checks := availabilityChecks("app-1", "avail-1", 3, "")
	if len(checks) != 0 {
		t.Fatalf("expected no checks, got %d (%v)", len(checks), checks)
	}
}

func TestAvailabilityChecks_UnverifiedWhenSkipped(t *testing.T) {
	checks := availabilityChecks("app-1", "", 0, "availability access unavailable")
	if !hasCheckID(checks, "availability.unverified") {
		t.Fatalf("expected availability.unverified check, got %v", checks)
	}
	if hasCheckID(checks, "availability.missing") {
		t.Fatalf("did not expect availability.missing when availability is unverified, got %v", checks)
	}
	if checks[0].Severity != SeverityWarning {
		t.Fatalf("expected warning severity, got %s", checks[0].Severity)
	}
}

func TestValidateIncludesPricingAndAvailabilitySkipWarnings(t *testing.T) {
	report := Validate(Input{
		AppID:                       "app-1",
		VersionID:                   "ver-1",
		PricingFetchSkipReason:      "pricing access unavailable",
		AvailabilityFetchSkipReason: "availability access unavailable",
	}, false)
	if !hasCheckID(report.Checks, "pricing.schedule.unverified") {
		t.Fatalf("expected pricing.schedule.unverified in unified validate, got %+v", report.Checks)
	}
	if !hasCheckID(report.Checks, "availability.unverified") {
		t.Fatalf("expected availability.unverified in unified validate, got %+v", report.Checks)
	}
}

func TestValidateAvailabilitySkipSuppressesPartialCoverageAndAddsCoverageSkipCheck(t *testing.T) {
	report := Validate(Input{
		AppID:                       "app-1",
		VersionID:                   "ver-1",
		AvailabilityFetchSkipReason: "availability access unavailable",
		PricingCoverageSkipReason:   "subscription pricing coverage verification was skipped because availability access was unavailable",
		AvailableTerritories:        175,
		AppAvailableTerritories:     []string{"USA", "CAN"},
		Subscriptions: []Subscription{
			{
				ID:               "sub-1",
				State:            "APPROVED",
				PriceCount:       1,
				PriceTerritories: []string{"USA"},
			},
		},
	}, false)

	if hasCheckID(report.Checks, "subscriptions.pricing.partial_territory_coverage") {
		t.Fatalf("did not expect partial coverage check when availability is unverified, got %+v", report.Checks)
	}
	if !hasCheckID(report.Checks, "subscriptions.pricing_coverage.unverified") {
		t.Fatalf("expected pricing coverage skip check in unified validate, got %+v", report.Checks)
	}
}

func TestValidateUsesPricingUniverseInsteadOfAppSaleAvailabilityForSubscriptionMatrix(t *testing.T) {
	report := Validate(Input{
		AppID:                   "app-1",
		VersionID:               "ver-1",
		AvailableTerritories:    1,
		AppAvailableTerritories: []string{"USA"},
		PricingTerritories:      []string{"USA", "CAN"},
		PricingTerritoryCount:   2,
		Subscriptions: []Subscription{
			{
				ID:               "sub-1",
				State:            "APPROVED",
				PriceCount:       1,
				PriceTerritories: []string{"USA"},
			},
		},
	}, false)

	if !hasCheckID(report.Checks, "subscriptions.pricing.partial_territory_coverage") {
		t.Fatalf("expected unified validate to require the full pricing universe, got %+v", report.Checks)
	}
}

func TestTerritoryContentStatusChecksReportTraderStatus(t *testing.T) {
	checks := territoryContentStatusChecks("avail-1", map[string][]string{
		"TRADER_STATUS_NOT_PROVIDED":        {"AUT", "FRA"},
		"TRADER_STATUS_VERIFICATION_FAILED": {"DEU"},
		"CANNOT_SELL":                       {"USA"},
	})
	if len(checks) != 1 || checks[0].ID != "availability.territories.trader_status" || checks[0].Severity != SeverityWarning {
		t.Fatalf("expected one trader status warning, got %v", checks)
	}
	for _, want := range []string{"TRADER_STATUS_NOT_PROVIDED (AUT, FRA)", "TRADER_STATUS_VERIFICATION_FAILED (DEU)"} {
		if !strings.Contains(checks[0].Message, want) {
			t.Fatalf("message %q is missing %q", checks[0].Message, want)
		}
	}
	if strings.Contains(checks[0].Message, "USA") || !strings.Contains(checks[0].Remediation, "asc web business trader-status") {
		t.Fatalf("unexpected trader status finding %+v", checks[0])
	}
	if got := territoryContentStatusChecks("avail-1", map[string][]string{"AVAILABLE": {"USA"}}); len(got) != 0 {
		t.Fatalf("expected no finding without a trader status, got %v", got)
	}
}

func TestTerritoryContentStatusChecksReportBlockingStatuses(t *testing.T) {
	tests := []struct {
		status, wantID, wantRemediation string
	}{
		{"MISSING_RATING", "availability.territories.missing_rating", "asc age-rating edit"},
		{"BRAZIL_REQUIRED_TAX_ID", "availability.territories.brazil_tax_id", "Business > Tax"},
		{"MISSING_GRN", "availability.territories.china_game_approval", "App Information"},
		{"UNVERIFIED_GRN", "availability.territories.china_game_approval", "App Information"},
		{"ICP_NUMBER_MISSING", "availability.territories.china_icp", "App Information"},
		{"ICP_NUMBER_INVALID", "availability.territories.china_icp", "App Information"},
		{"CANNOT_SELL_CASINO_WITHOUT_GRAC", "availability.territories.korea_rating", "--grac-rating-classification-number"},
		{"CANNOT_SELL_NINETEEN_PLUS_WITHOUT_GRAC", "availability.territories.korea_rating", "--grac-rating-classification-number"},
		{"CANNOT_SELL_SEVENTEEN_PLUS_APPS", "availability.territories.cannot_sell", "hold the pointer over"},
		{"BRAZIL_GAMBLING_NOT_VERIFIED", "availability.territories.cannot_sell", "hold the pointer over"},
		// A value Apple adds later still reports the restriction.
		{"CANNOT_SELL_SOMETHING_NEW", "availability.territories.cannot_sell", "hold the pointer over"},
	}
	for _, test := range tests {
		checks := territoryContentStatusChecks("avail-1", map[string][]string{test.status: {"BRA", "CHN"}})
		if len(checks) != 1 || checks[0].ID != test.wantID || checks[0].Severity != SeverityWarning {
			t.Errorf("%s: expected one %s warning, got %+v", test.status, test.wantID, checks)
			continue
		}
		if !strings.Contains(checks[0].Message, test.status+" (BRA, CHN)") || !strings.Contains(checks[0].Remediation, test.wantRemediation) {
			t.Errorf("%s: unexpected finding %+v", test.status, checks[0])
		}
	}
}

func TestTerritoryContentStatusChecksIgnoreInformationalStatuses(t *testing.T) {
	// An app that was never released reports CANNOT_SELL together with
	// AVAILABLE_FOR_SALE_UNRELEASED_APP in every territory (observed live).
	checks := territoryContentStatusChecks("avail-1", map[string][]string{
		"CANNOT_SELL":                       {"FRA", "USA"},
		"AVAILABLE_FOR_SALE_UNRELEASED_APP": {"FRA", "USA"},
		"AVAILABLE":                         {"DEU"},
		"AVAILABLE_FOR_PREORDER":            {"JPN"},
		"PREORDER_ON_UNRELEASED_APP":        {"JPN"},
		"PROCESSING_TO_AVAILABLE":           {"GBR"},
		"SOMETHING_NEW":                     {"ITA"},
	})
	if len(checks) != 0 {
		t.Fatalf("expected no findings for informational statuses, got %+v", checks)
	}
}

func TestTerritoryContentStatusChecksReportEachStatusOnce(t *testing.T) {
	checks := territoryContentStatusChecks("avail-1", map[string][]string{
		"CANNOT_SELL_CASINO_WITHOUT_GRAC": {"KOR"},
		"CANNOT_SELL_ADULT_ONLY":          {"CHN"},
	})
	if len(checks) != 2 || checks[0].ID != "availability.territories.korea_rating" || checks[1].ID != "availability.territories.cannot_sell" {
		t.Fatalf("expected a Korea finding then a sale restriction, got %+v", checks)
	}
	if strings.Contains(checks[1].Message, "KOR") || !strings.Contains(checks[1].Message, "1 territory") {
		t.Fatalf("a status matched by a specific rule must not repeat in the sale restriction: %+v", checks[1])
	}
}
