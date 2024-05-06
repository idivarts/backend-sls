package timehandler_test

import (
	"testing"

	"github.com/TrendsHub/th-backend/internal/models"
	timehandler "github.com/TrendsHub/th-backend/internal/time_handler"
)

func TestCalculateMessageDelay(t *testing.T) {

	conv := &models.Conversation{ThreadID: "thread_Dq5w7QFOluBlPtFsaEQgSlaX"}

	// Test case where the calculated time is greater than 30 minutes
	result, err := timehandler.CalculateMessageDelay(conv)
	if err != nil {
		t.Errorf("CalculateMessageDelay returned an error: %v", err)
	}
	if *result <= 0 || *result > 45*60 {
		t.Errorf("Invalid delay time calculated: %v", *result)
	}
	t.Log("Delay in seconds", *result)
	// Add more test cases as needed
}
