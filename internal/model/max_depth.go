package model

const defaultReentrantMaxDepth = 3

// resolveMaxDepth returns an effective max_depth.
// Priority: field.max_depth > relation.max_depth > defaultReentrantMaxDepth.
// The second return value indicates that the default value was used.
func resolveMaxDepth(fieldMax, relMax int) (int, bool) {
	if fieldMax > 0 {
		return fieldMax, false
	}
	if relMax > 0 {
		return relMax, false
	}
	return defaultReentrantMaxDepth, true
}
