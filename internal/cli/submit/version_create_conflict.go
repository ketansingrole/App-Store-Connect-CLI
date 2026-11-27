package submit

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/asc"
	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/cli/shared"
)

// releasedVersionStates are the states of a version App Store Connect has
// finished with: on the store, replaced by a newer one, or taken off sale.
var releasedVersionStates = map[string]struct{}{
	"READY_FOR_DISTRIBUTION":      {},
	"READY_FOR_SALE":              {},
	"PREORDER_READY_FOR_SALE":     {},
	"REPLACED_WITH_NEW_VERSION":   {},
	"REMOVED_FROM_SALE":           {},
	"DEVELOPER_REMOVED_FROM_SALE": {},
	"NOT_APPLICABLE":              {},
}

// renamableVersionStates are the states in which a version's number can still
// be changed: being prepared, or returned by App Review or the developer.
var renamableVersionStates = map[string]struct{}{
	"PREPARE_FOR_SUBMISSION": {},
	"DEVELOPER_REJECTED":     {},
	"REJECTED":               {},
	"METADATA_REJECTED":      {},
	"INVALID_BINARY":         {},
}

func appStoreVersionState(attributes asc.AppStoreVersionAttributes) string {
	if state := strings.TrimSpace(attributes.AppVersionState); state != "" {
		return state
	}
	return strings.TrimSpace(attributes.AppStoreState)
}

// ExplainVersionCreateConflict adds, to App Store Connect's refusal (409) to
// create version for platform, the version of that platform it still has in
// progress and the asc commands that finish it or give it this number. Apple's
// own message ("You cannot create a new version of the App in the current
// state") names neither. Any other error is returned as it is, and so is the
// refusal when the lookups fail or find no version in progress.
func ExplainVersionCreateConflict(ctx context.Context, client *asc.Client, appID, platform, version string, createErr error) error {
	if createErr == nil || !errors.Is(createErr, asc.ErrConflict) {
		return createErr
	}
	versions, err := shared.FetchAllAppStoreVersions(
		ctx,
		client,
		appID,
		asc.WithAppStoreVersionsPlatforms([]string{platform}),
		asc.Limit[asc.AppStoreVersionsOption](200),
	)
	if err != nil {
		return createErr
	}
	var pending *asc.Resource[asc.AppStoreVersionAttributes]
	for i := range versions {
		if _, released := releasedVersionStates[appStoreVersionState(versions[i].Attributes)]; !released {
			pending = &versions[i]
			break
		}
	}
	if pending == nil {
		return createErr
	}

	pendingVersion := strings.TrimSpace(pending.Attributes.VersionString)
	state := appStoreVersionState(pending.Attributes)
	rename := fmt.Sprintf("`asc versions update --version-id %q --version %q`", pending.ID, version)
	var hint string
	submissionID, err := findUnresolvedSubmissionForVersion(ctx, client, appID, platform, pending.ID)
	switch _, renamable := renamableVersionStates[state]; {
	case err == nil && submissionID != "":
		hint = shared.UnresolvedReviewSubmissionRemediation(appID, submissionID) +
			fmt.Sprintf(" Or give %s this version's number with %s, then run this command again.", pendingVersion, rename)
	case renamable:
		hint = fmt.Sprintf("Give %s this version's number with %s, then run this command again.", pendingVersion, rename)
	default:
		hint = fmt.Sprintf("See where it stands with `asc review status --app %q`.", appID)
	}
	return &shared.HintedError{
		Err: shared.WithDiagnostic(
			fmt.Errorf("%w; version %s (%s) of %s is still in progress", createErr, pendingVersion, state, platform),
			shared.DiagnosticResourceConflict,
			"",
		),
		Hint: hint,
	}
}
