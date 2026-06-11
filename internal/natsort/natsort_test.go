package natsort

import (
	"testing"
)

func TestLess(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{
			name: "numbers - less than",
			a:    "2",
			b:    "10",
			want: true,
		},
		{
			name: "numbers - equal",
			a:    "5",
			b:    "5",
			want: false,
		},
		{
			name: "channel names with numbers",
			a:    "Channel 2",
			b:    "Channel 10",
			want: true,
		},
		{
			name: "alphabetic comparison",
			a:    "abc",
			b:    "def",
			want: true,
		},
		{
			name: "number before text",
			a:    "123",
			b:    "abc",
			want: true,
		},
		{
			name: "empty strings",
			a:    "",
			b:    "",
			want: false,
		},
		{
			name: "first empty",
			a:    "",
			b:    "abc",
			want: true,
		},
		{
			// Lexicographic comparison would put "010" before "9".
			name: "leading zeros compare numerically",
			a:    "Channel 010",
			b:    "Channel 9",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Less(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("Less(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
