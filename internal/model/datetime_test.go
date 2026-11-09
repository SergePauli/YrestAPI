package model

import (
	"testing"
	"time"
)

func TestScanDatetimeLocalConversion(t *testing.T) {
	old := datetimeLocation
	t.Cleanup(func() { SetDatetimeLocation(old) })
	utc := time.Date(2026, 11, 9, 20, 30, 0, 0, time.UTC)
	nested := &DataPreset{Fields: []Field{{Source: "at", Type: "datetime"}}}
	child := &Model{}
	m := &Model{Relations: map[string]*ModelRelation{
		"child": {Type: "belongs_to", _ModelRef: child},
	}}
	preset := &DataPreset{Fields: []Field{
		{Source: "at", Type: "datetime"},
		{Source: "day", Type: "date"},
		{Source: "clock", Type: "time"},
		{Source: "empty", Type: "datetime"},
		{Source: "child", Type: "preset", _PresetRef: nested},
	}}
	am := &AliasMap{PathToAlias: map[string]string{"child": "t0"}}
	for _, enabled := range []bool{false, true} {
		SetDatetimeLocation(nil)
		want := "2026-11-09T20:30:00Z"
		if enabled {
			SetDatetimeLocation(time.FixedZone("local", 8*60*60))
			want = "2026-11-10T04:30:00+08:00"
		}
		rows := &stubRows{data: [][]any{{utc, utc, utc, nil, utc}}}
		items, err := m.ScanFlatRows(rows, preset, am)
		if err != nil {
			t.Fatal(err)
		}
		row := items[0]
		for _, value := range []any{row["at"], row["child"].(map[string]any)["at"]} {
			tm := value.(time.Time)
			if !tm.Equal(utc) || tm.Format(time.RFC3339) != want {
				t.Fatalf("unexpected datetime: %v", tm)
			}
		}
		if row["day"] != utc || row["clock"] != utc || row["empty"] != nil {
			t.Fatalf("other values changed: %v", row)
		}
	}
}
