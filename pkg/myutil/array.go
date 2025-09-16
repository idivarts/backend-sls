package myutil

func AppendUnique(slice []string, val string) ([]string, bool) {
	for _, item := range slice {
		if item == val {
			return slice, false
		}
	}
	return append(slice, val), true
}

func AppendUniqueWithMap(slice []string, val string) []string {
	m := make(map[string]struct{}, len(slice))
	for _, id := range slice {
		m[id] = struct{}{}
	}
	m[val] = struct{}{}

	slice = make([]string, 0, len(m))
	for id := range m {
		slice = append(slice, id)
	}
	return slice
}
