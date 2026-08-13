package sshinstall

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/owenewans/ulcer/internal/config"
	"github.com/owenewans/ulcer/internal/model"
)

var (
	imagePattern               = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._:/-]*[a-z0-9])?@sha256:[0-9a-f]{64}$`)
	repositoryComponentPattern = regexp.MustCompile(`^[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*$`)
)

func ValidImage(image string) bool {
	name, _, hasDigest := strings.Cut(image, "@sha256:")
	registry, repository, fullyQualified := strings.Cut(name, "/")
	if !hasDigest || !fullyQualified || (registry != "localhost" && !strings.ContainsAny(registry, ".:")) ||
		!imagePattern.MatchString(image) || strings.Count(image, "@") != 1 ||
		strings.Contains(image, "://") || strings.Contains(image, "//") {
		return false
	}
	for _, component := range strings.Split(repository, "/") {
		if !repositoryComponentPattern.MatchString(component) {
			return false
		}
	}
	return true
}

func Available(configuration config.Host) bool {
	return ValidImage(configuration.InstanceImage) && validEndpoint(configuration.PublicGRPC) && validServerName(configuration.PublicName)
}

func renderInstanceEnv(id, name, endpoint, serverName string) ([]byte, error) {
	if !safeUUID(id) || model.ValidateInstanceName(name) != nil || !validEndpoint(endpoint) || !validServerName(serverName) {
		return nil, fmt.Errorf("invalid instance runtime configuration")
	}
	return []byte(fmt.Sprintf(
		"ULCER_INSTANCE_ID=%s\nULCER_INSTANCE_NAME=%s\nULCER_HOST_GRPC=%s\nULCER_HOST_SERVER_NAME=%s\nULCER_INSTANCE_DATA_DIR=/var/lib/ulcer-instance\nULCER_INSTANCE_CERT=/run/ulcer-identity/instance.crt\nULCER_INSTANCE_KEY=/run/ulcer-identity/instance.key\nULCER_INSTANCE_CA=/run/ulcer-identity/ca.crt\n",
		id, name, endpoint, serverName,
	)), nil
}

func renderUnit(id, image string) ([]byte, error) {
	if !safeUUID(id) || !ValidImage(image) {
		return nil, fmt.Errorf("invalid instance service configuration")
	}
	directory := "/etc/ulcer/instances/" + id
	container := "ulcer-instance-" + id
	return []byte(fmt.Sprintf(`[Unit]
Description=Ulcer INSTANCE agent %s
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/bin/podman run --rm --name=%s --pull=never --network=bridge --read-only --user=65532:65532 --security-opt=no-new-privileges --cap-drop=all --pids-limit=128 --memory=256m --cpus=1 --tmpfs=/tmp:rw,noexec,nosuid,nodev,size=16m --tmpfs=/var/lib/ulcer-instance:rw,noexec,nosuid,nodev,size=32m,uid=65532,gid=65532,mode=0700 --env-file=%s/instance.env --volume=%s:/run/ulcer-identity:ro %s
ExecStop=-/usr/bin/podman stop --time=10 %s
ExecStopPost=-/usr/bin/podman rm --force %s
Restart=on-failure
RestartSec=5s
TimeoutStartSec=300s
TimeoutStopSec=30s
TasksMax=160
MemoryMax=384M
CPUQuota=100%%
NoNewPrivileges=yes

[Install]
WantedBy=multi-user.target
`, id, container, directory, directory, image, container, container)), nil
}

func safeUUID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for index, character := range id {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if character != '-' {
				return false
			}
			continue
		}
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func safeEnvironmentValue(value string) bool {
	if value == "" || len(value) > 255 {
		return false
	}
	for _, character := range value {
		if character < 0x21 || character > 0x7e || character == '#' || character == '=' {
			return false
		}
	}
	return true
}

func validEndpoint(endpoint string) bool {
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil || !validServerName(host) {
		return false
	}
	number, err := strconv.Atoi(port)
	return err == nil && number > 0 && number <= 65535
}

func validServerName(name string) bool {
	if !safeEnvironmentValue(name) || strings.ContainsAny(name, "[]/") {
		return false
	}
	_, _, err := normalizeHost(name)
	return err == nil
}
