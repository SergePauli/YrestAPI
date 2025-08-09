package model

import (
	"fmt"

	"github.com/Masterminds/squirrel"
)

func (m *Model) BuildCountQuery(filters map[string]interface{}) (squirrel.SelectBuilder, error) {
	sb := squirrel.SelectBuilder{}.PlaceholderFormat(squirrel.Dollar)
	sb = sb.From(fmt.Sprintf("%s AS main", m.Table))

	var filterKeys []string
	for key := range filters {
		filterKeys = append(filterKeys, key)
	}
	// Определим, какие JOIN-ы нужны — на основе ключей в filters
	requiredJoins, err := m.DetectJoins(filterKeys, nil, nil)
	if err != nil {
		return sb, err
	}
	

	hasDistinct := false
	for i := 0; i < len(requiredJoins); i++ {
    join := requiredJoins[i]  
		onClause := join.On
    if join.Where != "" {
        onClause = fmt.Sprintf("(%s) AND (%s)", join.On, join.Where)
    }
		sb = sb.LeftJoin(fmt.Sprintf("%s AS %s ON %s", join.Table, join.Alias, onClause ))
		if join.Distinct {
			hasDistinct = true
		}
	}
	// Подставляем SELECT COUNT(...) в зависимости от наличия has_many
	if hasDistinct {
		sb = sb.Column(fmt.Sprintf("COUNT(DISTINCT %s.id)", "main"))
	} else {
		sb = sb.Column("COUNT(*)")
	}

	wherePart, err := m.buildWhereClause(filters, requiredJoins)
	if err != nil {
		return sb, err
	}
	if wherePart != nil {
		sb = sb.Where(wherePart)
	}
	return sb, nil
}