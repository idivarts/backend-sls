package payments

import (
	"fmt"
	"time"
)

// UpdateTransferHold updates Razorpay Route transfer settlement hold (PATCH /v1/transfers/:id).
//
// If holdDays <= 0, the hold is released (on_hold=false) so settlement can proceed per Razorpay rules.
//
// If holdDays > 0, the transfer remains on hold until on_hold_until, set to holdDays calendar
// days from now in the local timezone (same semantics as time.Now().AddDate(0, 0, holdDays)).
func UpdateTransferHold(transferID string, holdDays int) (map[string]interface{}, error) {
	if transferID == "" {
		return nil, fmt.Errorf("empty transfer id")
	}

	var data map[string]interface{}
	if holdDays <= 0 {
		data = map[string]interface{}{
			"on_hold": false,
		}
	} else {
		until := time.Now().AddDate(0, 0, holdDays).Unix()
		data = map[string]interface{}{
			"on_hold":       true,
			"on_hold_until": until,
		}
	}

	return Client.Transfer.Edit(transferID, data, nil)
}
