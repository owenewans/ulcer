package sshinstall

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const networkTimeout = 15 * time.Second

var (
	ErrInvalidTarget   = errors.New("invalid SSH target")
	ErrTargetBlocked   = errors.New("SSH target address is not allowed")
	ErrHostKeyMismatch = errors.New("SSH host key does not match the confirmed fingerprint")
	ErrInvalidHostKey  = errors.New("invalid SSH host key fingerprint")
	errHostKeyCaptured = errors.New("SSH host key captured")
)

type HostKey struct {
	Algorithm   string `json:"algorithm"`
	Fingerprint string `json:"fingerprint"`
}

type resolvedTarget struct {
	host    string
	address string
}

func ProbeHostKey(ctx context.Context, host string, port int) (HostKey, error) {
	ctx, cancel := context.WithTimeout(ctx, networkTimeout)
	defer cancel()
	target, err := resolveTarget(ctx, host, port)
	if err != nil {
		return HostKey{}, err
	}
	return probeResolvedHostKey(ctx, target)
}

func probeResolvedHostKey(ctx context.Context, target resolvedTarget) (HostKey, error) {
	connection, err := dialTarget(ctx, target)
	if err != nil {
		return HostKey{}, fmt.Errorf("SSH handshake failed")
	}
	defer connection.Close()
	stopCancellation := context.AfterFunc(ctx, func() { _ = connection.Close() })
	defer stopCancellation()

	var captured ssh.PublicKey
	config := &ssh.ClientConfig{
		User: "root",
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			captured = key
			return errHostKeyCaptured
		},
	}
	_, _, _, err = ssh.NewClientConn(connection, target.host, config)
	if !errors.Is(err, errHostKeyCaptured) || captured == nil {
		return HostKey{}, fmt.Errorf("SSH handshake failed")
	}
	return HostKey{Algorithm: captured.Type(), Fingerprint: ssh.FingerprintSHA256(captured)}, nil
}

func resolveTarget(ctx context.Context, host string, port int) (resolvedTarget, error) {
	host, literal, err := normalizeHost(host)
	if err != nil {
		return resolvedTarget{}, err
	}
	if port < 1 || port > 65535 {
		return resolvedTarget{}, fmt.Errorf("%w: port must be between 1 and 65535", ErrInvalidTarget)
	}

	addresses := []netip.Addr{literal}
	if !literal.IsValid() {
		addresses, err = net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil || len(addresses) == 0 {
			return resolvedTarget{}, fmt.Errorf("%w: host could not be resolved", ErrInvalidTarget)
		}
	}
	for index := range addresses {
		addresses[index] = addresses[index].Unmap()
		if !allowedTargetAddress(addresses[index]) {
			return resolvedTarget{}, ErrTargetBlocked
		}
	}
	address := addresses[0]
	return resolvedTarget{
		host:    net.JoinHostPort(host, strconv.Itoa(port)),
		address: net.JoinHostPort(address.String(), strconv.Itoa(port)),
	}, nil
}

func normalizeHost(host string) (string, netip.Addr, error) {
	if host == "" || strings.TrimSpace(host) != host || len(host) > 253 || strings.ContainsAny(host, "/\\%@") {
		return "", netip.Addr{}, ErrInvalidTarget
	}
	if strings.HasPrefix(host, "[") || strings.HasSuffix(host, "]") {
		if !strings.HasPrefix(host, "[") || !strings.HasSuffix(host, "]") {
			return "", netip.Addr{}, ErrInvalidTarget
		}
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	if address, err := netip.ParseAddr(host); err == nil {
		return address.String(), address.Unmap(), nil
	}
	if strings.Contains(host, ":") || allNumericHost(host) {
		return "", netip.Addr{}, ErrInvalidTarget
	}
	name := strings.TrimSuffix(strings.ToLower(host), ".")
	if name == "" {
		return "", netip.Addr{}, ErrInvalidTarget
	}
	for _, label := range strings.Split(name, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", netip.Addr{}, ErrInvalidTarget
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
				return "", netip.Addr{}, ErrInvalidTarget
			}
		}
	}
	return name, netip.Addr{}, nil
}

func allNumericHost(host string) bool {
	for _, character := range host {
		if (character < '0' || character > '9') && character != '.' {
			return false
		}
	}
	return true
}

func allowedTargetAddress(address netip.Addr) bool {
	if !address.IsValid() || address.IsUnspecified() || address.IsLoopback() ||
		address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsMulticast() {
		return false
	}
	if address.Is6() && address.Is4In6() {
		return false
	}
	if address.Is4() {
		bytes := address.As4()
		return bytes[0] != 0 && address != netip.MustParseAddr("255.255.255.255")
	}
	return true
}

func dialTarget(ctx context.Context, target resolvedTarget) (net.Conn, error) {
	connection, err := (&net.Dialer{Timeout: networkTimeout, KeepAlive: 30 * time.Second}).DialContext(ctx, "tcp", target.address)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(networkTimeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := connection.SetDeadline(deadline); err != nil {
		connection.Close()
		return nil, err
	}
	return connection, nil
}

func fingerprintCallback(fingerprint string) (ssh.HostKeyCallback, error) {
	encoded, found := strings.CutPrefix(fingerprint, "SHA256:")
	if !found {
		return nil, ErrInvalidHostKey
	}
	digest, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil || len(digest) != 32 || base64.RawStdEncoding.EncodeToString(digest) != encoded {
		return nil, ErrInvalidHostKey
	}
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		actual := ssh.FingerprintSHA256(key)
		if subtle.ConstantTimeCompare([]byte(actual), []byte(fingerprint)) != 1 {
			return ErrHostKeyMismatch
		}
		return nil
	}, nil
}
