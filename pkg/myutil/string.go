package myutil

import "strings"

func DerefString(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}

func ContainsIgnoreCase(base, search string) bool {
	return strings.Contains(strings.ToLower(base), strings.ToLower(search))
}

func DerefInt64(s *int64) int64 {
	if s != nil {
		return *s
	}
	return 0
}

func IntPtr(i int) *int {
	return &i
}

func BoolPtr(b bool) *bool {
	return &b
}
