package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

type Host struct {
	HTTPAddr      string
	GRPCAddr      string
	DataDir       string
	PublicName    string
	PublicGRPC    string
	InstanceImage string
	SetupToken    string
}

func HostFromEnv() Host {
	publicName := publicHost(envOr("ULCER_PUBLIC_NAME", "localhost"))
	return Host{
		HTTPAddr:      envOr("ULCER_HTTP_ADDR", "127.0.0.1:8080"),
		GRPCAddr:      envOr("ULCER_GRPC_ADDR", "127.0.0.1:8443"),
		DataDir:       envOr("ULCER_DATA_DIR", "./data/host"),
		PublicName:    publicName,
		PublicGRPC:    envOr("ULCER_PUBLIC_GRPC", publicGRPCDefault(publicName)),
		InstanceImage: os.Getenv("ULCER_INSTANCE_IMAGE"),
		SetupToken:    os.Getenv("ULCER_SETUP_TOKEN"),
	}
}

func publicGRPCDefault(publicName string) string {
	host := publicHost(publicName)
	if host == "" {
		host = "localhost"
	}
	return net.JoinHostPort(host, "8443")
}

func publicHost(value string) string {
	host := strings.TrimSpace(value)
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	return host
}

type Instance struct {
	ID         string
	Name       string
	Host       string
	ServerName string
	DataDir    string
	CertFile   string
	KeyFile    string
	CAFile     string
}

func InstanceFromEnv() (Instance, error) {
	dataDir := envOr("ULCER_INSTANCE_DATA_DIR", "./data/instance")
	config := Instance{
		ID:         os.Getenv("ULCER_INSTANCE_ID"),
		Name:       envOr("ULCER_INSTANCE_NAME", "local-instance"),
		Host:       envOr("ULCER_HOST_GRPC", "localhost:8443"),
		ServerName: envOr("ULCER_HOST_SERVER_NAME", "localhost"),
		DataDir:    dataDir,
		CertFile:   envOr("ULCER_INSTANCE_CERT", filepath.Join(dataDir, "instance.crt")),
		KeyFile:    envOr("ULCER_INSTANCE_KEY", filepath.Join(dataDir, "instance.key")),
		CAFile:     envOr("ULCER_INSTANCE_CA", filepath.Join(dataDir, "ca.crt")),
	}
	if config.ID == "" {
		return Instance{}, fmt.Errorf("ULCER_INSTANCE_ID is required")
	}
	return config, nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
