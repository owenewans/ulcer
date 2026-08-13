package config

import "testing"

func TestPublicGRPCDefault(t *testing.T) {
	tests := map[string]string{
		"example.com":    "example.com:8443",
		"192.0.2.10":     "192.0.2.10:8443",
		"2001:db8::10":   "[2001:db8::10]:8443",
		"[2001:db8::10]": "[2001:db8::10]:8443",
	}
	for publicName, expected := range tests {
		if actual := publicGRPCDefault(publicName); actual != expected {
			t.Errorf("publicGRPCDefault(%q) = %q, want %q", publicName, actual, expected)
		}
	}
}

func TestHostFromEnvMetadata(t *testing.T) {
	t.Setenv("ULCER_PUBLIC_NAME", "[2001:db8::10]")
	t.Setenv("ULCER_PUBLIC_GRPC", "")
	t.Setenv("ULCER_INSTANCE_IMAGE", "example/instance@sha256:digest")
	configuration := HostFromEnv()
	if configuration.PublicGRPC != "[2001:db8::10]:8443" {
		t.Fatalf("PublicGRPC = %q", configuration.PublicGRPC)
	}
	if configuration.PublicName != "2001:db8::10" {
		t.Fatalf("PublicName = %q", configuration.PublicName)
	}
	if configuration.InstanceImage != "example/instance@sha256:digest" {
		t.Fatalf("InstanceImage = %q", configuration.InstanceImage)
	}
}
