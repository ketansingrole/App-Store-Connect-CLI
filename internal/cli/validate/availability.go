package validate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/asc"
	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/cli/shared"
)

var fetchAvailableTerritoryDetailsFn = fetchAvailableTerritoryDetails

// fetchAvailableTerritoryDetails also returns the territories Apple reports
// under each content status, for every territory whether or not it is
// available.
func fetchAvailableTerritoryDetails(ctx context.Context, client *asc.Client, appID string) (string, []string, int, map[string][]string, error) {
	ctx = withReadinessRequestGate(ctx)
	availabilityID := ""
	availableTerritories := 0
	decodedAvailableTerritories := 0
	territoryIDs := make(map[string]struct{})
	contentStatuses := make(map[string][]string)

	availabilityResp, err := doReadinessRequest(ctx, func(requestCtx context.Context) (*asc.AppAvailabilityV2Response, error) {
		return client.GetAppAvailabilityV2(requestCtx, appID)
	})
	if err != nil {
		if shared.IsAppAvailabilityMissing(err) {
			return "", nil, 0, nil, nil
		}
		return "", nil, 0, nil, fmt.Errorf("failed to fetch app availability: %w", err)
	}

	availabilityID = strings.TrimSpace(availabilityResp.Data.ID)
	if availabilityID == "" {
		return "", nil, 0, nil, nil
	}

	nextURL := ""
	for {
		territoryResp, requestErr := doReadinessRequest(ctx, func(requestCtx context.Context) (*asc.TerritoryAvailabilitiesResponse, error) {
			if strings.TrimSpace(nextURL) != "" {
				return client.GetTerritoryAvailabilities(requestCtx, availabilityID, asc.NextURL[asc.TerritoryAvailabilitiesOption](nextURL))
			}
			return client.GetTerritoryAvailabilities(requestCtx, availabilityID, asc.Limit[asc.TerritoryAvailabilitiesOption](200))
		})
		err = requestErr
		if err != nil {
			return availabilityID, nil, availableTerritories, nil, fmt.Errorf("failed to fetch territory availabilities: %w", err)
		}

		for _, territoryAvailability := range territoryResp.Data {
			collectTerritoryContentStatuses(contentStatuses, territoryAvailability)
			if territoryAvailability.Attributes.Available {
				availableTerritories++
				territoryID, err := territoryAvailabilityTerritoryID(territoryAvailability.Relationships)
				if err != nil || strings.TrimSpace(territoryID) == "" {
					continue
				}
				decodedAvailableTerritories++
				territoryIDs[strings.TrimSpace(territoryID)] = struct{}{}
			}
		}

		nextURL = strings.TrimSpace(territoryResp.Links.Next)
		if nextURL == "" {
			break
		}
	}

	ids := make([]string, 0, len(territoryIDs))
	for territoryID := range territoryIDs {
		ids = append(ids, territoryID)
	}
	slices.Sort(ids)
	for status := range contentStatuses {
		slices.Sort(contentStatuses[status])
	}
	if availableTerritories > 0 && decodedAvailableTerritories != availableTerritories {
		return availabilityID, nil, availableTerritories, contentStatuses, nil
	}

	return availabilityID, ids, availableTerritories, contentStatuses, nil
}

func collectTerritoryContentStatuses(statuses map[string][]string, territoryAvailability asc.Resource[asc.TerritoryAvailabilityAttributes]) {
	if len(territoryAvailability.Attributes.ContentStatuses) == 0 {
		return
	}
	territoryID, err := territoryAvailabilityTerritoryID(territoryAvailability.Relationships)
	territoryID = strings.TrimSpace(territoryID)
	if err != nil || territoryID == "" {
		territoryID = strings.TrimSpace(territoryAvailability.ID)
	}
	for _, status := range territoryAvailability.Attributes.ContentStatuses {
		status = strings.TrimSpace(status)
		if status != "" && territoryID != "" {
			statuses[status] = append(statuses[status], territoryID)
		}
	}
}

func territoryAvailabilityTerritoryID(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}

	var relationships asc.TerritoryAvailabilityRelationships
	if err := json.Unmarshal(raw, &relationships); err != nil {
		return "", fmt.Errorf("decode territory availability relationships: %w", err)
	}
	return strings.TrimSpace(relationships.Territory.Data.ID), nil
}

func availabilityCheckSkipReason(err error) (string, bool) {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "Subscription availability coverage verification was skipped because the App Store Connect availability endpoints timed out", true
	case errors.Is(err, asc.ErrForbidden) || asc.IsUnauthorized(err):
		return "Subscription availability coverage verification was skipped because this App Store Connect account cannot read app availability territories", true
	case asc.IsRetryable(err):
		return "Subscription availability coverage verification was skipped because the App Store Connect availability endpoints were temporarily unavailable or rate limited", true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return "Subscription availability coverage verification was skipped because the App Store Connect availability endpoints could not be reached", true
	}

	return "", false
}

func pricingTerritoryCheckSkipReason(err error) (string, bool) {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "Subscription pricing matrix verification was skipped because the App Store pricing territories endpoint timed out", true
	case errors.Is(err, asc.ErrForbidden) || asc.IsUnauthorized(err):
		return "Subscription pricing matrix verification was skipped because this App Store Connect account cannot read pricing territories", true
	case asc.IsRetryable(err):
		return "Subscription pricing matrix verification was skipped because the App Store pricing territories endpoint was temporarily unavailable or rate limited", true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return "Subscription pricing matrix verification was skipped because the App Store pricing territories endpoint could not be reached", true
	}
	return "", false
}
