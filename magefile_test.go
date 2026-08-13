//go:build mage

package main

import "testing"

func TestValidBuildValue(t *testing.T) {
	for _, value := range []string{"development", "v1.2.3", "feature/ref", "refs/tags/v1.2.3", "0123456789abcdef"} {
		if !validBuildValue(value) {
			t.Errorf("valid build value %q rejected", value)
		}
	}
	for _, value := range []string{"", "bad value", "bad\nvalue", "$(command)", "value;command", "value=value"} {
		if validBuildValue(value) {
			t.Errorf("invalid build value %q accepted", value)
		}
	}
}
