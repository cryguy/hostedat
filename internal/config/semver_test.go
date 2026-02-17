package config

import "testing"

func TestParseSemver_Valid(t *testing.T) {
	tests := []struct {
		input string
		want  Semver
	}{
		{"0.1.0", Semver{0, 1, 0}},
		{"1.2.3", Semver{1, 2, 3}},
		{"v1.2.3", Semver{1, 2, 3}},
		{"10.20.30", Semver{10, 20, 30}},
		{"0.0.0", Semver{0, 0, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseSemver(tt.input)
			if err != nil {
				t.Fatalf("ParseSemver(%q): %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseSemver(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseSemver_Invalid(t *testing.T) {
	tests := []string{
		"dev",
		"1.2",
		"1",
		"abc",
		"",
		"1.2.x",
		"v",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := ParseSemver(input)
			if err == nil {
				t.Errorf("ParseSemver(%q) should have failed", input)
			}
		})
	}
}

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "0.9.9", 1},
		{"0.9.9", "1.0.0", -1},
		{"1.2.0", "1.1.9", 1},
		{"1.1.0", "1.1.1", -1},
		{"0.1.0", "0.1.0", 0},
		{"2.0.0", "1.99.99", 1},
	}

	for _, tt := range tests {
		a, _ := ParseSemver(tt.a)
		b, _ := ParseSemver(tt.b)
		got := CompareSemver(a, b)
		if got != tt.want {
			t.Errorf("CompareSemver(%s, %s) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestSemverAtLeast(t *testing.T) {
	tests := []struct {
		version string
		minimum string
		want    bool
	}{
		{"1.0.0", "1.0.0", true},
		{"1.0.1", "1.0.0", true},
		{"0.9.0", "1.0.0", false},
		{"dev", "1.0.0", true}, // dev builds pass
		{"1.0.0", "dev", true}, // unparseable minimum passes
		{"abc", "1.0.0", true}, // unparseable version passes
		{"0.1.0", "0.1.0", true},
		{"0.2.0", "0.1.0", true},
		{"0.0.9", "0.1.0", false},
	}

	for _, tt := range tests {
		got := SemverAtLeast(tt.version, tt.minimum)
		if got != tt.want {
			t.Errorf("SemverAtLeast(%q, %q) = %v, want %v", tt.version, tt.minimum, got, tt.want)
		}
	}
}
