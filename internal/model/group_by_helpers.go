package model

import "strings"

func isHasManyPath(root *Model, path string) bool {
	if root == nil || strings.TrimSpace(path) == "" {
		return false
	}
	curr := root
	segs := strings.Split(path, ".")
	for i, seg := range segs {
		rel := curr.Relations[seg]
		if rel == nil {
			return false
		}
		if i == len(segs)-1 {
			return rel.Type == "has_many"
		}
		if rel._ModelRef == nil {
			return false
		}
		curr = rel._ModelRef
	}
	return false
}

func containsString(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}
