package validation

import (
	"fmt"
	"slices"
	"strings"
)

func pricingChecks(appID string, priceScheduleID string, basePriceMissing bool, baseTerritory string, skipReason string) []CheckResult {
	if strings.TrimSpace(skipReason) != "" {
		return []CheckResult{{
			ID:           "pricing.schedule.unverified",
			Severity:     SeverityWarning,
			Field:        "appPriceSchedule",
			ResourceType: "app",
			ResourceID:   strings.TrimSpace(appID),
			Message:      "could not verify app price schedule",
			Remediation:  strings.TrimSpace(skipReason),
		}}
	}
	if strings.TrimSpace(priceScheduleID) != "" {
		if !basePriceMissing {
			return nil
		}
		territory := strings.ToUpper(strings.TrimSpace(baseTerritory))
		message := "app has no price set"
		remediationTerritory := "USA"
		if territory != "" {
			message = fmt.Sprintf("app has no price set for base territory %s", territory)
			remediationTerritory = territory
		}
		return []CheckResult{{
			ID:           "pricing.base_price.missing",
			Severity:     SeverityError,
			Field:        "manualPrices",
			ResourceType: "appPriceSchedule",
			ResourceID:   strings.TrimSpace(priceScheduleID),
			Message:      message,
			Remediation:  fmt.Sprintf("Set a price (Free counts): asc pricing schedule create --app %s --free --base-territory %q", strings.TrimSpace(appID), remediationTerritory),
		}}
	}
	return []CheckResult{
		{
			ID:           "pricing.schedule.missing",
			Severity:     SeverityError,
			Field:        "appPriceSchedule",
			ResourceType: "app",
			ResourceID:   strings.TrimSpace(appID),
			Message:      "app price schedule is missing",
			Remediation:  "Set the price the developer chooses, Free included (list price points with asc pricing price-points)",
		},
	}
}

func availabilityChecks(appID string, availabilityID string, availableTerritories int, skipReason string) []CheckResult {
	if strings.TrimSpace(skipReason) != "" {
		return []CheckResult{{
			ID:           "availability.unverified",
			Severity:     SeverityWarning,
			Field:        "appAvailabilityV2",
			ResourceType: "app",
			ResourceID:   strings.TrimSpace(appID),
			Message:      "could not verify app availability",
			Remediation:  strings.TrimSpace(skipReason),
		}}
	}
	if strings.TrimSpace(availabilityID) == "" {
		return []CheckResult{
			{
				ID:           "availability.missing",
				Severity:     SeverityError,
				Field:        "appAvailabilityV2",
				ResourceType: "app",
				ResourceID:   strings.TrimSpace(appID),
				Message:      "app availability is missing",
				Remediation:  "Make the app available in at least one territory",
			},
		}
	}

	if availableTerritories > 0 {
		return nil
	}

	return []CheckResult{
		{
			ID:           "availability.territories.none",
			Severity:     SeverityError,
			Field:        "territoryAvailabilities",
			ResourceType: "appAvailabilityV2",
			ResourceID:   strings.TrimSpace(availabilityID),
			Message:      fmt.Sprintf("no available territories configured (available=%d)", availableTerritories),
			Remediation:  "Enable at least one territory",
		},
	}
}

// territoryContentStatusRule groups territory content statuses that Apple
// reports for one cause into a single finding. A status belongs to the first
// rule that lists it or, failing that, to the first rule whose prefix it has.
type territoryContentStatusRule struct {
	id          string
	statuses    []string
	prefix      string
	subject     string
	remediation string
}

// territoryContentStatusRules cover the statuses that keep the app off sale in
// a territory. Plain CANNOT_SELL is not one of them: an app that was never
// released reports it in every territory next to
// AVAILABLE_FOR_SALE_UNRELEASED_APP, and App Store Connect gives the reason
// in a separate status. Available, pre-order, and processing statuses are
// informational.
var territoryContentStatusRules = []territoryContentStatusRule{
	{
		id: "availability.territories.trader_status",
		statuses: []string{
			"TRADER_STATUS_NOT_PROVIDED",
			"TRADER_STATUS_VERIFICATION_FAILED",
			"TRADER_STATUS_VERIFICATION_STATUS_MISSING",
		},
		subject:     "an EU Digital Services Act trader status problem",
		remediation: "Have the Account Holder or an Admin provide or fix the trader status in App Store Connect > Business > Digital Services Act, then check it with `asc web business trader-status`",
	},
	{
		id:          "availability.territories.missing_rating",
		statuses:    []string{"MISSING_RATING"},
		subject:     "a missing age rating",
		remediation: "Set the age rating: review it with `asc age-rating view --app APP_ID` and answer the questionnaire with `asc age-rating edit`",
	},
	{
		id:          "availability.territories.brazil_tax_id",
		statuses:    []string{"BRAZIL_REQUIRED_TAX_ID"},
		subject:     "missing Brazilian tax information",
		remediation: "Have the Account Holder or an Admin submit the Brazilian tax information in App Store Connect > Business > Tax",
	},
	{
		id:          "availability.territories.china_game_approval",
		statuses:    []string{"MISSING_GRN", "UNVERIFIED_GRN"},
		subject:     "a missing or unverified China mainland game approval number",
		remediation: "Provide the game approval number and supporting documents from China's National Press and Publication Administration under App Information in App Store Connect",
	},
	{
		id:          "availability.territories.china_icp",
		statuses:    []string{"ICP_NUMBER_MISSING", "ICP_NUMBER_INVALID"},
		subject:     "a missing or invalid China mainland ICP Filing Number",
		remediation: "Provide an ICP Filing Number that matches your MIIT filing, and metadata that matches it, under App Information in App Store Connect",
	},
	{
		id: "availability.territories.korea_rating",
		statuses: []string{
			"CANNOT_SELL_CASINO_WITHOUT_GRAC",
			"CANNOT_SELL_NINETEEN_PLUS_WITHOUT_GRAC",
			"CANNOT_SELL_FREQUENT_INTENSE_WITHOUT_GRAC",
			"CANNOT_SELL_FREQUENT_WITHOUT_GRAC",
		},
		subject:     "a missing Korea GRAC Rating Classification Number",
		remediation: "Get a Rating Classification Number from Korea's Game Rating and Administration Committee and set it with `asc age-rating edit --app APP_ID --grac-rating-classification-number NUMBER`",
	},
	{
		id:          "availability.territories.cannot_sell",
		statuses:    []string{"BRAZIL_GAMBLING_NOT_VERIFIED"},
		prefix:      "CANNOT_SELL_",
		subject:     "a sale restriction",
		remediation: "Open Pricing and Availability in App Store Connect and hold the pointer over each territory's status for Apple's reason",
	},
}

// territoryContentStatusChecks reports the territory content statuses Apple
// returns with the app's territory availabilities.
func territoryContentStatusChecks(availabilityID string, statuses map[string][]string) []CheckResult {
	byRule := make([][]string, len(territoryContentStatusRules))
	for status := range statuses {
		if index := territoryContentStatusRuleIndex(status); index >= 0 && len(statuses[status]) > 0 {
			byRule[index] = append(byRule[index], status)
		}
	}
	var checks []CheckResult
	for index, rule := range territoryContentStatusRules {
		matched := byRule[index]
		if len(matched) == 0 {
			continue
		}
		slices.Sort(matched)
		parts := make([]string, 0, len(matched))
		territories := make(map[string]struct{})
		for _, status := range matched {
			ids := statuses[status]
			parts = append(parts, fmt.Sprintf("%s (%s)", status, strings.Join(ids, ", ")))
			for _, id := range ids {
				territories[id] = struct{}{}
			}
		}
		checks = append(checks, CheckResult{
			ID:           rule.id,
			Severity:     SeverityWarning,
			Field:        "territoryAvailabilities",
			ResourceType: "appAvailabilityV2",
			ResourceID:   strings.TrimSpace(availabilityID),
			Message:      fmt.Sprintf("Apple reports %s for %d %s: %s", rule.subject, len(territories), pluralTerritories(len(territories)), strings.Join(parts, "; ")),
			Remediation:  rule.remediation,
		})
	}
	return checks
}

func territoryContentStatusRuleIndex(status string) int {
	for index, rule := range territoryContentStatusRules {
		for _, listed := range rule.statuses {
			if status == listed {
				return index
			}
		}
	}
	for index, rule := range territoryContentStatusRules {
		if rule.prefix != "" && strings.HasPrefix(status, rule.prefix) {
			return index
		}
	}
	return -1
}

func pluralTerritories(count int) string {
	if count == 1 {
		return "territory"
	}
	return "territories"
}
