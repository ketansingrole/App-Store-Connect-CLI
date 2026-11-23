package validation

import (
	"fmt"
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
// reports for one cause into a single finding.
type territoryContentStatusRule struct {
	id          string
	statuses    []string
	subject     string
	remediation string
}

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
}

// territoryContentStatusChecks reports the territory content statuses Apple
// returns with the app's territory availabilities.
func territoryContentStatusChecks(availabilityID string, statuses map[string][]string) []CheckResult {
	var checks []CheckResult
	for _, rule := range territoryContentStatusRules {
		var parts []string
		territories := make(map[string]struct{})
		for _, status := range rule.statuses {
			ids := statuses[status]
			if len(ids) == 0 {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s (%s)", status, strings.Join(ids, ", ")))
			for _, id := range ids {
				territories[id] = struct{}{}
			}
		}
		if len(parts) == 0 {
			continue
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

func pluralTerritories(count int) string {
	if count == 1 {
		return "territory"
	}
	return "territories"
}
