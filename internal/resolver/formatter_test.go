package resolver

import (
	"testing"
)

func TestApplyFormatter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		fmt  string
		row  map[string]any
		want string
	}{
		{
			name: "direct key no index",
			fmt:  "{name}",
			row:  map[string]any{"name": "John"},
			want: "John",
		},
		{
			name: "nested path no index",
			fmt:  "{naming.surname}",
			row: map[string]any{
				"naming": map[string]any{"surname": "Иванов"},
			},
			want: "Иванов",
		},
		{
			name: "single index within bounds (ASCII)",
			fmt:  "{name}[1]",
			row:  map[string]any{"name": "John"},
			want: "o",
		},
		{
			name: "range within bounds (ASCII)",
			fmt:  "{name}[1..3]",
			row:  map[string]any{"name": "John"},
			want: "oh",
		},
		{
			name: "index out of bounds returns empty",
			fmt:  "{name}[10]",
			row:  map[string]any{"name": "John"},
			want: "",
		},
		{
			name: "range end clipped to length",
			fmt:  "{name}[2..10]",
			row:  map[string]any{"name": "John"},
			want: "hn",
		},
		{
			name: "range i>=j returns empty",
			fmt:  "{name}[2..2]",
			row:  map[string]any{"name": "John"},
			want: "",
		},
		{
			name: "multibyte single index (UTF-8 runes)",
			fmt:  "{naming.surname}[0]",
			row:  map[string]any{"naming": map[string]any{"surname": "Иванов"}},
			want: "И",
		},
		{
			name: "multibyte range slice",
			fmt:  "{naming.surname}[0..2]",
			row:  map[string]any{"naming": map[string]any{"surname": "Иванов"}},
			want: "Ив",
		},
		{
			name: "missing path returns empty",
			fmt:  "{missing}",
			row:  map[string]any{"name": "John"},
			want: "",
		},
		{
			name: "multiple tokens with spaces",
			fmt:  "{naming.surname} {naming.name}[0] {naming.patrname}[0..1]",
			row: map[string]any{
				"naming": map[string]any{
					"surname":  "Иванов",
					"name":     "Сергей",
					"patrname": "Петрович",
				},
			},
			want: "Иванов С П",
		},
		{
			name: "direct key takes precedence over nested",
			fmt:  "{naming.surname}",
			row: map[string]any{
				"naming.surname": "PRECEDENCE",
				"naming":         map[string]any{"surname": "ignored"},
			},
			want: "PRECEDENCE",
		},
		{
			name: "non-string value formats via Sprintf",
			fmt:  "{age}",
			row:  map[string]any{"age": 42},
			want: "42",
		},
		{
			name: "surrounding text preserved",
			fmt:  "Full: {name}, Age: {age}",
			row:  map[string]any{"name": "Ann", "age": 7},
			want: "Full: Ann, Age: 7",
		},
		{
    	name: "ternary used=true + first letter",
    	fmt:  `{? used ? "+" : "-"} {name}[0]`,
    	row:  map[string]any{"used": true, "name": "Иван"},
    	want: "+ И",
		},
		{
    	name: "ternary used=false - first letter",
    	fmt:  `{? used ? "+" : "-"} {name}[0]`,
    	row:  map[string]any{"used": false, "name": "Иван"},
    	want: "- И",
		},
		{
    	name: "ternary numeric branches (then number, else number)",
    	fmt:  `{? age >= 18 ? 1 : 0}`,
    	row:  map[string]any{"age": 20},
    	want: "1",
		},
		{
    	name: "ternary numeric branches (else chosen)",
    	fmt:  `{? age >= 18 ? 1 : 0}`,
    	row:  map[string]any{"age": 16},
    	want: "0",
		},
		{
    	name: "ternary with numeric then + token afterwards",
    	fmt:  `{? age >= 18 ? 42 : 7} {name}[0]`,
    	row:  map[string]any{"age": 19, "name": "Иван"},
    	want: "42 И",
		},
		{
    	name: "ternary boolean branches (true/false literals)",
    	fmt:  `{? active == true ? true : false}`,
    	row:  map[string]any{"active": true},
    	want: "true",
		},
		{
    	name: "ternary boolean branches (else false)",
    	fmt:  `{? active == true ? true : false}`,
    	row:  map[string]any{"active": false},
    	want: "false",
		},
		{
    	name: "ternary else null literal",
    	fmt:  `{? middle != null ? "{middle}[0]." : null}`,
    	row:  map[string]any{"middle": nil},
    	want: "", // null -> печатаем как пустую строку
		},
		{
    	name: "ternary then null literal (condition true)",
    	fmt:  `{? middle == "" ? null : "{middle}[0]."}`,
    	row:  map[string]any{"middle": ""},
    	want: "", // then=null -> пусто
		},
		{
    	name: "ternary string literals (unquote works)",
    	fmt:  `{? status == "ok" ? "✔" : "✖"}`,
    	row:  map[string]any{"status": "ok"},
    	want: "✔",
		},
		{
    	name: "ternary string single-quoted literals",
    	fmt:  `{? status != 'ok' ? 'bad' : 'good'}`,
    	row:  map[string]any{"status": "ok"},
    	want: "good",
		},
		{
    	name: "ternary shorthand truthy (number nonzero)",
    	fmt:  `{? score ? "A" : "B"}`,
    	row:  map[string]any{"score": 10},
    	want: "A",
		},
		{
    	name: "ternary shorthand falsy (number zero)",
    	fmt:  `{? score ? "A" : "B"}`,
    	row:  map[string]any{"score": 0},
    	want: "B",
		},
		{
    	name: "ternary shorthand falsy (empty string)",
    	fmt:  `{? tag ? "X" : "Y"}`,
    	row:  map[string]any{"tag": ""},
    	want: "Y",
		},
		{
    	name: "ternary shorthand truthy (non-empty string)",
    	fmt:  `{? tag ? "X" : "Y"}`,
    	row:  map[string]any{"tag": "go"},
    	want: "X",
		},
		{
    	name: "ternary numeric compare with string numeric right",
    	fmt:  `{? age >= "18" ? "adult" : "minor"}`,
    	row:  map[string]any{"age": 18},
    	want: "adult",
		},
		{
    	name: "ternary boolean compare with string 'true'",
    	fmt:  `{? active == "true" ? "ON" : "OFF"}`,
    	row:  map[string]any{"active": true},
    	want: "ON",
		},
		{
    	name: "multiple ternaries and tokens mixed",
    	fmt:  `{? used ? "+" : "-"} {name}[0] {? age > 30 ? "(30+)" : ""}`,
    	row:  map[string]any{"used": true, "name": "Иван", "age": 31},
    	want: "+ И (30+)",
		},
		{
    	name: "nested ternary inside branch",
    	fmt:  `{? used ? "{? age >= 18 ? "adult" : "minor"}" : "-"}`,
    	row:  map[string]any{"used": true, "age": 20},
    	want: "adult",
		},
		{
    	name: "nested ternary inside branch (minor)",
    	fmt:  `{? used ? "{? age >= 18 ? "adult" : "minor"}" : "-"}`,
    	row:  map[string]any{"used": true, "age": 15},
    	want: "minor",
		},
		{
    	name: "nested ternary else branch",
    	fmt:  `{? used ? "{? age >= 18 ? "adult" : "minor"}" : "-"}`,
    	row:  map[string]any{"used": false, "age": 25},
    	want: "-",
		},

	}

	for _, tc := range tests {		
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ApplyFormatterTestShim(tc.fmt, tc.row)
			if got != tc.want {
				t.Fatalf("applyFormatter(%q) = %q, want %q", tc.fmt, got, tc.want)
			}
		})
	}
	t.Run("unmatched_brace_preserved", func(t *testing.T) {
    got := ApplyFormatterTestShim("{name", map[string]any{"name":"Ann"})
    if got != "{name" { t.Fatalf("got %q", got) }
	})	

	t.Run("range_j_less_than_i_empty", func(t *testing.T) {
    got := ApplyFormatterTestShim("{name}[3..1]", map[string]any{"name":"John"})
    if got != "" { t.Fatalf("got %q", got) }
	})
}
