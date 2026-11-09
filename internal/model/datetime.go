package model

import "time"

// datetimeLocation is configured once at service startup. Nil preserves DB output.
var datetimeLocation *time.Location

func SetDatetimeLocation(location *time.Location) {
	datetimeLocation = location
}

// PostgreSQL timestamp without time zone values are stored as UTC by convention.
func localDatetime(value any) any {
	if datetimeLocation == nil {
		return value
	}
	if t, ok := value.(time.Time); ok {
		return t.In(datetimeLocation)
	}
	return value
}
