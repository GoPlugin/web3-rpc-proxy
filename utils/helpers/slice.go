package helpers

func MergeSlicesBy[T comparable](iteratee func(a, b T) T, unique func(element T) string, arrays ...[]T) []T {
	exists := make(map[string]T)

	for _, array := range arrays {
		for _, item := range array {
			id := unique(item)
			if existing, found := exists[id]; found {
				exists[id] = iteratee(existing, item)
			} else {
				exists[id] = item
			}
		}
	}

	mergedArray := make([]T, 0, len(exists))
	for key := range exists {
		mergedArray = append(mergedArray, exists[key])
	}

	return mergedArray
}
