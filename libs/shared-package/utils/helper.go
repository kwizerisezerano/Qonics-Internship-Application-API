package utils

import "reflect"

// AvailableSourceTable lists the tables that may own a file through the
// polymorphic source_table / source_table_id pair (leazi-style allow-list).
var AvailableSourceTable = []string{
	"applications",
}

// In_array reports whether value is present in slice. It mirrors the leazi
// helper of the same name so file-source validation stays consistent.
func In_array(slice any, value any) bool {
	s := reflect.ValueOf(slice)

	if s.Kind() != reflect.Slice {
		panic("slice value: is not a slice")
	}
	for i := 0; i < s.Len(); i++ {
		if reflect.DeepEqual(s.Index(i).Interface(), value) {
			return true
		}
	}
	return false
}
