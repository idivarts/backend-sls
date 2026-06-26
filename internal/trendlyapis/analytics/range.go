package analytics

import (
	"strconv"
	"time"

	"github.com/idivarts/backend-sls/pkg/facebook"
)

// ParseRange normalizes a query string into a supported Range, defaulting to 28d.
func ParseRange(s string) Range {
	switch Range(s) {
	case Range7d, Range28d, Range90d:
		return Range(s)
	default:
		return Range28d
	}
}

// Days returns the number of days the range spans.
func (r Range) Days() int {
	switch r {
	case Range7d:
		return 7
	case Range90d:
		return 90
	default:
		return 28
	}
}

// SinceUntil returns Unix-second strings for the window ending at now.
func (r Range) SinceUntil(now time.Time) (since string, until string) {
	return strconv.FormatInt(now.AddDate(0, 0, -r.Days()).Unix(), 10),
		strconv.FormatInt(now.Unix(), 10)
}

// FBDatePreset maps the range onto the closest Facebook date_preset.
func (r Range) FBDatePreset() facebook.FBInsightDatePreset {
	switch r {
	case Range7d:
		return facebook.FBDatePresetLast7d
	case Range90d:
		return facebook.FBDatePresetLast90d
	default:
		return facebook.FBDatePresetLast28d
	}
}
