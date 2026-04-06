package webhook

// TransferEntity is the transfer object embedded in transfer.* webhook payloads.
type TransferEntity struct {
	ID    string                 `json:"id"`
	Notes map[string]interface{} `json:"notes"`
}
