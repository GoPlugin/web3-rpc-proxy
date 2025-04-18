package endpoint

import (
	"github.com/duke-git/lancet/v2/maputil"
)

func Merge(vs ...*Endpoint) *Endpoint {
	result := make([]map[string]any, len(vs), cap(vs))
	for i := 0; i < len(vs); i++ {
		result[i] = vs[i].state
	}
	state := maputil.Merge(result...)

	return &Endpoint{state: state}
}
