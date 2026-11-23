package validate

import (
	"context"
	"fmt"
	"strings"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/validation"
	webcore "github.com/rudrankriyam/App-Store-Connect-CLI/internal/web"
)

const businessAppStoreConnectURL = "https://appstoreconnect.apple.com/business"

const traderStatusFixRemediation = "Have the Account Holder or an Admin complete it in App Store Connect > Business > Digital Services Act; Apple requires it to make apps available in the EU. Check it with `asc web business trader-status`"

// collectTraderStatusDeepCheck reports the EU Digital Services Act trader
// status. It passes only when the account declared its status and every
// legal entity that covers the EU shows Active, the label App Store Connect
// itself chooses; an in-progress or unrecognized label stays unverified.
func collectTraderStatusDeepCheck(ctx context.Context, client deepWebClient) (validation.DeepCheck, *validation.CheckResult) {
	requestCtx, cancel := readinessRequestContext(ctx)
	defer cancel()
	info, err := client.GetSellerInfo(requestCtx)
	if err != nil || info == nil {
		check := unverifiedWebCheck(validation.DeepCheckDSATraderStatus, "EU trader status could not be verified through the cached Apple web session", businessAppStoreConnectURL)
		return check, unverifiedFinding("deep.dsa.trader_status.unverified", check.Message, "Retry after confirming the cached Apple web session can read App Store Connect Business, or inspect it with `asc web business trader-status`", "traderStatus", "", check.Resolution)
	}

	switch info.TraderDeclaration {
	case webcore.TraderDeclarationNotDeclared:
		return blockedTraderStatus("dsa.trader_status.not_declared", "EU trader status is not declared")
	case webcore.TraderDeclarationTrader, webcore.TraderDeclarationNotTrader:
	default:
		return unverifiedTraderStatus("Apple returned an unknown EU trader declaration")
	}
	if len(info.LegalEntities) == 0 {
		return unverifiedTraderStatus("Apple returned an EU trader declaration without legal entities")
	}

	var needsAction, notConfirmed []string
	euCovered := 0
	for _, entity := range info.LegalEntities {
		if !entity.IsEUCovered {
			continue
		}
		euCovered++
		label := string(entity.Status())
		switch entity.Status() {
		case webcore.TraderStatusActive:
			continue
		case webcore.TraderStatusActionNeeded, webcore.TraderStatusRejected, webcore.TraderStatusNotDeclared:
			needsAction = append(needsAction, traderEntityDescription(entity.LegalEntityID, label))
		default:
			if label == "" {
				label = strings.TrimSpace(entity.VerificationStatus)
			}
			if label == "" {
				label = "unknown"
			}
			notConfirmed = append(notConfirmed, traderEntityDescription(entity.LegalEntityID, label))
		}
	}
	switch {
	case len(needsAction) > 0:
		return blockedTraderStatus("dsa.trader_status.action_needed", "EU trader status needs action for legal entity "+strings.Join(needsAction, ", "))
	case len(notConfirmed) > 0:
		return unverifiedTraderStatus("EU trader status is not confirmed for legal entity " + strings.Join(notConfirmed, ", "))
	case euCovered == 0:
		return validation.DeepCheck{
			ID:      validation.DeepCheckDSATraderStatus,
			Status:  validation.DeepStatusNotApplicable,
			Source:  validation.DeepSourceWebSession,
			Message: "No legal entity is covered by the EU Digital Services Act",
		}, nil
	default:
		return validation.DeepCheck{
			ID:      validation.DeepCheckDSATraderStatus,
			Status:  validation.DeepStatusPassed,
			Source:  validation.DeepSourceWebSession,
			Message: "EU trader status is declared and Active for every legal entity in the EU",
		}, nil
	}
}

func traderEntityDescription(legalEntityID, label string) string {
	legalEntityID = strings.TrimSpace(legalEntityID)
	if legalEntityID == "" {
		legalEntityID = "with no ID"
	}
	return fmt.Sprintf("%s (%s)", legalEntityID, label)
}

func blockedTraderStatus(findingID, message string) (validation.DeepCheck, *validation.CheckResult) {
	resolution := &validation.Resolution{
		Fixability:         validation.FixabilityManual,
		AppStoreConnectURL: businessAppStoreConnectURL,
	}
	return validation.DeepCheck{
		ID:         validation.DeepCheckDSATraderStatus,
		Status:     validation.DeepStatusBlocked,
		Source:     validation.DeepSourceWebSession,
		Message:    message,
		Resolution: resolution,
	}, &validation.CheckResult{
		ID:           findingID,
		Severity:     validation.SeverityError,
		Message:      message,
		Remediation:  traderStatusFixRemediation,
		ResourceType: "traderStatus",
		Resolution:   resolution,
	}
}

func unverifiedTraderStatus(message string) (validation.DeepCheck, *validation.CheckResult) {
	check := unverifiedWebCheck(validation.DeepCheckDSATraderStatus, message, businessAppStoreConnectURL)
	return check, unverifiedFinding("deep.dsa.trader_status.unverified", message, "Review it in App Store Connect > Business > Digital Services Act, or check again later with `asc web business trader-status`", "traderStatus", "", check.Resolution)
}

// isTraderStatusContractMessage reports whether an App Store Connect contract
// message is the EU trader status alert rather than an agreement alert.
func isTraderStatusContractMessage(lowerText string) bool {
	return strings.Contains(lowerText, "trader status") || strings.Contains(lowerText, "digital services act")
}
