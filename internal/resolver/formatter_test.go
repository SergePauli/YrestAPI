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
	}

	for _, tc := range tests {
		tc := tc
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
