package agentclient

import (
	"fmt"
	"time"
)

// verifySourceFreshness checks optional source expiries at the actual live
// evaluation time. Signature verification and schemas are established first.
func verifySourceFreshness(now time.Time, skew time.Duration, records ...[]byte) error {
	for _, raw := range records {
		if len(raw) == 0 {
			continue
		}
		record, err := parseCanonicalObject(raw, "signed source record")
		if err != nil {
			return err
		}
		provenance, ok := objectValue(record["provenance"])
		if !ok {
			return fmt.Errorf("signed source provenance missing")
		}
		observedRaw, _ := provenance["observedAt"].(string)
		observed, err := time.Parse(time.RFC3339, observedRaw)
		if err != nil || observed.After(now.Add(skew)) {
			return fmt.Errorf("signed source originates in the future or has invalid observation time")
		}
		if expiryRaw, exists := provenance["expiresAt"]; exists {
			value, _ := expiryRaw.(string)
			expiry, err := time.Parse(time.RFC3339, value)
			if err != nil || !expiry.After(observed) || !now.Before(expiry) {
				return fmt.Errorf("signed source is expired or has invalid ordering")
			}
		}
	}
	return nil
}
