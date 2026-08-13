package model

import (
	"strings"
	"testing"
)

func TestValidateInstanceName(t *testing.T) {
	for _, valid := range []string{"a", "edge-01", "a1", strings.Repeat("a", 63)} {
		if err := ValidateInstanceName(valid); err != nil {
			t.Errorf("valid name %q rejected: %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "Edge", "edge_01", "edge 01", "-edge", "edge-", "edge.example", strings.Repeat("a", 64)} {
		if err := ValidateInstanceName(invalid); err == nil {
			t.Errorf("invalid name %q accepted", invalid)
		}
	}
}
