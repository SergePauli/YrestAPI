package model

import (
	"strings"
	"testing"
)

func TestDetectJoins_BelongsToChain(t *testing.T) {
	// address.area цепочка
	area := &Model{Table: "areas", Relations: map[string]*ModelRelation{}}
	aliasMap := &AliasMap{
			PathToAlias: map[string]string{
				"address":       "t0",
				"address.area":  "t1",
			},
			AliasToPath: map[string]string{
				"t0": "address",
				"t1": "address.area",
			},
		}
	address := &Model{
		Table: "addresses",
		Relations: map[string]*ModelRelation{
			"area": {Type: "belongs_to", FK: "area_id", PK: "id", _ModelRef: area},
		},
	}

	root := &Model{
		Table: "contragent_addresses",
		Relations: map[string]*ModelRelation{
			"address": {Type: "belongs_to", FK: "address_id", PK: "id", _ModelRef: address},
		},
		
	}

	// presetFields из ScanPresetFields (фактически то, что ты уже логируешь)
	presetFields := []string{"address", "address.area"}

	joins, err := root.DetectJoins(aliasMap,nil, nil, presetFields)
	if err != nil {
		t.Fatalf("DetectJoins error: %v", err)
	}

	var gotAddr, gotArea bool
	for _, j := range joins {
		switch j.Table {
		case "addresses":
			// main.address_id = t0.id
			if j.Alias != "t0" || !strings.Contains(j.On, "main.address_id = t0.id") {
				t.Fatalf("addresses join mismatch: %+v", j)
			}
			gotAddr = true
		case "areas":
			// t0.area_id = t1.id
			if j.Alias != "t1" || !strings.Contains(j.On, "t0.area_id = t1.id") {
				t.Fatalf("areas join mismatch: %+v", j)
			}
			gotArea = true
		}
	}

	if !gotAddr || !gotArea {
		t.Fatalf("expected joins for addresses and areas, got: %+v", joins)
	}
}
