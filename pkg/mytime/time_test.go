package mytime_test

import (
	"testing"
	"time"

	"github.com/idivarts/backend-sls/pkg/mytime"
)

func TestTime(t *testing.T) {
	t.Log("Time", mytime.FormatPrettyIST(time.Now()))
}
