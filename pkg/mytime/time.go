package mytime

import (
	"fmt"
	"time"
)

func FormatPrettyIST(t time.Time) string {
	loc, _ := time.LoadLocation("Asia/Kolkata")
	t = t.In(loc)

	day := t.Day()
	suffix := "th"
	if day%10 == 1 && day != 11 {
		suffix = "st"
	} else if day%10 == 2 && day != 12 {
		suffix = "nd"
	} else if day%10 == 3 && day != 13 {
		suffix = "rd"
	}

	return fmt.Sprintf("%d%s %s %d - %s IST",
		day, suffix, t.Format("Jan"), t.Year(), t.Format("3:04pm"))
}
