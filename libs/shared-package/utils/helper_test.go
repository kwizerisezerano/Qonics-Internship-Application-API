package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestHumanizeStatus covers the display formatting of machine status values:
// snake_case is converted to a title-cased, human-readable label.
func TestHumanizeStatus(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   string
	}{
		{name: "single word", status: "submitted", want: "Submitted"},
		{name: "snake case", status: "under_review", want: "Under Review"},
		{name: "multiple words", status: "interview_scheduled_soon", want: "Interview Scheduled Soon"},
		{name: "already capitalised", status: "Accepted", want: "Accepted"},
		{name: "empty", status: "", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, HumanizeStatus(test.status))
		})
	}
}
