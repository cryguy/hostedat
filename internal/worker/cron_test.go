package worker

import (
	"testing"
	"time"
)

func TestCronMatches(t *testing.T) {
	tests := []struct {
		name  string
		expr  string
		time  time.Time
		match bool
	}{
		{"every minute", "* * * * *", time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC), true},
		{"exact match", "30 12 1 1 1", time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC), true},
		{"no match minute", "0 12 1 1 *", time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC), false},
		{"step */5 match", "*/5 * * * *", time.Date(2024, 1, 1, 12, 15, 0, 0, time.UTC), true},
		{"step */5 no match", "*/5 * * * *", time.Date(2024, 1, 1, 12, 13, 0, 0, time.UTC), false},
		{"range match", "0-30 * * * *", time.Date(2024, 1, 1, 12, 15, 0, 0, time.UTC), true},
		{"range no match", "0-10 * * * *", time.Date(2024, 1, 1, 12, 15, 0, 0, time.UTC), false},
		{"comma list match", "0,15,30,45 * * * *", time.Date(2024, 1, 1, 12, 15, 0, 0, time.UTC), true},
		{"comma list no match", "0,30,45 * * * *", time.Date(2024, 1, 1, 12, 15, 0, 0, time.UTC), false},
		{"invalid field count", "* * *", time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC), false},
		{"weekday Sunday=0", "* * * * 0", time.Date(2024, 1, 7, 12, 0, 0, 0, time.UTC), true},
		{"step */15 at 0", "*/15 * * * *", time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC), true},
		{"step */15 at 45", "*/15 * * * *", time.Date(2024, 1, 1, 12, 45, 0, 0, time.UTC), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cronMatches(tt.expr, tt.time)
			if got != tt.match {
				t.Errorf("cronMatches(%q, %v) = %v, want %v", tt.expr, tt.time, got, tt.match)
			}
		})
	}
}

func TestValidateCron(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{"valid every minute", "* * * * *", false},
		{"valid specific", "30 12 1 1 1", false},
		{"valid step", "*/5 * * * *", false},
		{"valid range", "0-30 * * * *", false},
		{"valid comma", "0,15,30 * * * *", false},
		{"valid combo", "0,30 */2 * * 1-5", false},
		{"too few fields", "* * *", true},
		{"too many fields", "* * * * * *", true},
		{"minute out of range", "60 * * * *", true},
		{"hour out of range", "* 24 * * *", true},
		{"day out of range", "* * 32 * *", true},
		{"month out of range", "* * * 13 *", true},
		{"weekday out of range", "* * * * 7", true},
		{"invalid step", "*/0 * * * *", true},
		{"invalid value", "* abc * * *", true},
		{"invalid range reversed", "* * 10-5 * *", true},
		{"day zero", "* * 0 * *", true},
		{"month zero", "* * * 0 *", true},
		{"empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCron(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCron(%q) error = %v, wantErr %v", tt.expr, err, tt.wantErr)
			}
		})
	}
}
