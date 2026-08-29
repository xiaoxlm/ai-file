package split

import (
	"reflect"
	"testing"
)

func TestParagraphs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{name: "empty", input: "", expected: []string{}},
		{name: "whitespace only", input: " \n\t\n ", expected: []string{}},
		{name: "single paragraph", input: "hello world", expected: []string{"hello world"}},
		{name: "multiple blank lines", input: "a\n\n\n\nb", expected: []string{"a", "b"}},
		{name: "crlf", input: "a\r\n\r\nb", expected: []string{"a", "b"}},
		{name: "trim blocks", input: " a \n\n b ", expected: []string{"a", "b"}},
		{name: "three paragraphs", input: "p1\n\np2\n\np3", expected: []string{"p1", "p2", "p3"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Paragraphs(tt.input); !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("Paragraphs(%q) = %#v, want %#v", tt.input, got, tt.expected)
			}
		})
	}
}
