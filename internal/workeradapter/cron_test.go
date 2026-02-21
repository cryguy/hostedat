package workeradapter

import (
	"testing"
	"time"
)

func TestCronMatches(t *testing.T) {
	tests := []struct {
		name string
		expr string
		time time.Time
		want bool
	}{
		{
			name: "every minute",
			expr: "* * * * *",
			time: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "exact minute match",
			expr: "30 * * * *",
			time: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "exact minute no match",
			expr: "15 * * * *",
			time: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			want: false,
		},
		{
			name: "exact hour and minute",
			expr: "30 10 * * *",
			time: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "every 5 minutes match",
			expr: "*/5 * * * *",
			time: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "every 5 minutes no match",
			expr: "*/5 * * * *",
			time: time.Date(2024, 1, 15, 10, 31, 0, 0, time.UTC),
			want: false,
		},
		{
			name: "day of month match",
			expr: "0 0 15 * *",
			time: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "month match",
			expr: "0 0 1 6 *",
			time: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "day of week match (Monday=1)",
			expr: "0 0 * * 1",
			time: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), // Monday
			want: true,
		},
		{
			name: "day of week no match",
			expr: "0 0 * * 1",
			time: time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC), // Tuesday
			want: false,
		},
		{
			name: "comma separated match",
			expr: "0,15,30,45 * * * *",
			time: time.Date(2024, 1, 15, 10, 15, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "comma separated no match",
			expr: "0,15,30,45 * * * *",
			time: time.Date(2024, 1, 15, 10, 20, 0, 0, time.UTC),
			want: false,
		},
		{
			name: "range match",
			expr: "* 9-17 * * *",
			time: time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "range no match",
			expr: "* 9-17 * * *",
			time: time.Date(2024, 1, 15, 20, 0, 0, 0, time.UTC),
			want: false,
		},
		{
			name: "invalid expression too few fields",
			expr: "* * *",
			time: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			want: false,
		},
		{
			name: "invalid expression too many fields",
			expr: "* * * * * *",
			time: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cronMatches(tt.expr, tt.time)
			if got != tt.want {
				t.Errorf("cronMatches(%q, %v) = %v, want %v", tt.expr, tt.time, got, tt.want)
			}
		})
	}
}

func TestFieldMatches(t *testing.T) {
	tests := []struct {
		name  string
		field string
		value int
		want  bool
	}{
		{"wildcard", "*", 42, true},
		{"exact match", "42", 42, true},
		{"exact no match", "10", 42, false},
		{"step match", "*/7", 42, true},     // 42 % 7 == 0
		{"step no match", "*/7", 43, false}, // 43 % 7 != 0
		{"step zero", "*/0", 5, false},      // invalid step
		{"step invalid", "*/abc", 5, false},
		{"comma match first", "1,5,10", 1, true},
		{"comma match middle", "1,5,10", 5, true},
		{"comma match last", "1,5,10", 10, true},
		{"comma no match", "1,5,10", 7, false},
		{"range match low", "5-10", 5, true},
		{"range match mid", "5-10", 7, true},
		{"range match high", "5-10", 10, true},
		{"range no match below", "5-10", 3, false},
		{"range no match above", "5-10", 12, false},
		{"invalid value", "abc", 5, false},
		{"invalid range low", "abc-10", 5, false},
		{"invalid range high", "5-abc", 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fieldMatches(tt.field, tt.value)
			if got != tt.want {
				t.Errorf("fieldMatches(%q, %d) = %v, want %v", tt.field, tt.value, got, tt.want)
			}
		})
	}
}
