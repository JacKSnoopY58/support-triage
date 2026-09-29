package businesslogic

import "support-triage-service/model"

// FilterFAQIDs keeps only IDs from the knowledge-base results supplied to the model.
func FilterFAQIDs(ids []string, hits []model.KBHit) []string {
	allowed := map[string]bool{}
	for _, hit := range hits {
		allowed[hit.ID] = true
	}
	result := []string{}
	for _, id := range ids {
		if allowed[id] {
			result = append(result, id)
		}
	}
	return result
}
