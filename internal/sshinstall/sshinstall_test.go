package sshinstall

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/owenewans/ulcer/internal/config"
	"golang.org/x/crypto/ssh"
)

const testID = "01234567-89ab-cdef-0123-456789abcdef"

func TestTargetAddressPolicy(t *testing.T) {
	tests := []struct {
		address string
		allowed bool
	}{
		{"0.0.0.0", false},
		{"127.0.0.1", false},
		{"169.254.1.1", false},
		{"224.0.0.1", false},
		{"255.255.255.255", false},
		{"::", false},
		{"::1", false},
		{"::ffff:127.0.0.1", false},
		{"fe80::1", false},
		{"ff02::1", false},
		{"10.0.0.10", true},
		{"192.168.1.10", true},
		{"fd00::10", true},
		{"203.0.113.10", true},
		{"2001:db8::10", true},
	}
	for _, test := range tests {
		t.Run(test.address, func(t *testing.T) {
			if allowed := allowedTargetAddress(netip.MustParseAddr(test.address)); allowed != test.allowed {
				t.Fatalf("allowedTargetAddress(%q) = %v, want %v", test.address, allowed, test.allowed)
			}
		})
	}
}

func TestNormalizeHostAndResolveLiteral(t *testing.T) {
	for _, host := range []string{"edge.example.com", "192.168.1.10", "[fd00::10]"} {
		if _, _, err := normalizeHost(host); err != nil {
			t.Errorf("normalizeHost(%q): %v", host, err)
		}
	}
	for _, host := range []string{"", " edge.example.com", "edge/example", "example.com:22", "1.2.3", "-edge.example"} {
		if _, _, err := normalizeHost(host); !errors.Is(err, ErrInvalidTarget) {
			t.Errorf("normalizeHost(%q) error = %v, want ErrInvalidTarget", host, err)
		}
	}
	target, err := resolveTarget(context.Background(), "192.168.1.10", 2222)
	if err != nil {
		t.Fatal(err)
	}
	if target.host != "192.168.1.10:2222" || target.address != target.host {
		t.Fatalf("unexpected resolved target: %+v", target)
	}
	if _, err := resolveTarget(context.Background(), "127.0.0.1", 22); !errors.Is(err, ErrTargetBlocked) {
		t.Fatalf("loopback error = %v, want ErrTargetBlocked", err)
	}
	if _, err := resolveTarget(context.Background(), "192.168.1.10", 0); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("invalid port error = %v, want ErrInvalidTarget", err)
	}
}

func TestFingerprintCallbackRejectsMismatch(t *testing.T) {
	serverSigner := newSigner(t)
	confirmedSigner := newSigner(t)
	callback, err := fingerprintCallback(ssh.FingerprintSHA256(confirmedSigner.PublicKey()))
	if err != nil {
		t.Fatal(err)
	}
	if err := callback("host", &net.TCPAddr{}, serverSigner.PublicKey()); !errors.Is(err, ErrHostKeyMismatch) {
		t.Fatalf("callback error = %v, want ErrHostKeyMismatch", err)
	}
	if _, err := fingerprintCallback("SHA256:not-base64"); !errors.Is(err, ErrInvalidHostKey) {
		t.Fatalf("invalid fingerprint error = %v, want ErrInvalidHostKey", err)
	}
}

func TestConnectSurfacesHostKeyMismatch(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverConfig := &ssh.ServerConfig{NoClientAuth: true}
	serverConfig.AddHostKey(newSigner(t))
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		_, _, _, _ = ssh.NewServerConn(connection, serverConfig)
	}()

	callback, err := fingerprintCallback(ssh.FingerprintSHA256(newSigner(t).PublicKey()))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	address := listener.Addr().String()
	_, closeClient, err := connect(ctx, resolvedTarget{host: address, address: address}, "root", callback, ssh.Password("unused"))
	closeClient()
	if !errors.Is(err, ErrHostKeyMismatch) {
		t.Fatalf("connect error = %v, want ErrHostKeyMismatch", err)
	}
	listener.Close()
	<-serverDone
}

func TestProbeResolvedHostKeyNeedsNoAuthentication(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	hostSigner := newSigner(t)
	serverConfig := &ssh.ServerConfig{
		PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) {
			return nil, errors.New("authentication must not be attempted")
		},
	}
	serverConfig.AddHostKey(hostSigner)
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		_, _, _, _ = ssh.NewServerConn(connection, serverConfig)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	address := listener.Addr().String()
	hostKey, err := probeResolvedHostKey(ctx, resolvedTarget{host: address, address: address})
	if err != nil {
		t.Fatal(err)
	}
	if hostKey.Algorithm != hostSigner.PublicKey().Type() || hostKey.Fingerprint != ssh.FingerprintSHA256(hostSigner.PublicKey()) {
		t.Fatalf("unexpected host key: %+v", hostKey)
	}
	listener.Close()
	<-serverDone
}

func TestAuthenticationValidation(t *testing.T) {
	if _, err := authenticationMethod(Request{}); !errors.Is(err, ErrInvalidAuth) {
		t.Fatalf("empty auth error = %v", err)
	}
	if _, err := authenticationMethod(Request{Password: []byte("password"), PrivateKeyPEM: []byte("key")}); !errors.Is(err, ErrInvalidAuth) {
		t.Fatalf("multiple auth error = %v", err)
	}
	if _, err := authenticationMethod(Request{Password: []byte("password"), PrivateKeyPassword: []byte("passphrase")}); !errors.Is(err, ErrInvalidAuth) {
		t.Fatalf("orphan passphrase error = %v", err)
	}
	if _, err := authenticationMethod(Request{Password: []byte("password")}); err != nil {
		t.Fatalf("password auth: %v", err)
	}

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	plainBlock, err := ssh.MarshalPrivateKey(privateKey, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authenticationMethod(Request{PrivateKeyPEM: pem.EncodeToMemory(plainBlock)}); err != nil {
		t.Fatalf("private key auth: %v", err)
	}
	encryptedBlock, err := ssh.MarshalPrivateKeyWithPassphrase(privateKey, "test", []byte("passphrase"))
	if err != nil {
		t.Fatal(err)
	}
	encrypted := pem.EncodeToMemory(encryptedBlock)
	if _, err := authenticationMethod(Request{PrivateKeyPEM: encrypted, PrivateKeyPassword: []byte("passphrase")}); err != nil {
		t.Fatalf("encrypted private key auth: %v", err)
	}
	if _, err := authenticationMethod(Request{PrivateKeyPEM: encrypted, PrivateKeyPassword: []byte("wrong")}); !errors.Is(err, ErrInvalidAuth) {
		t.Fatalf("wrong passphrase error = %v", err)
	}
}

func TestRenderRuntimeFiles(t *testing.T) {
	image := "ghcr.io/owenewans/ulcer-instance@sha256:" + strings.Repeat("a", 64)
	if !ValidImage(image) {
		t.Fatal("digest-pinned image was rejected")
	}
	for _, invalid := range []string{
		"", "ghcr.io/owenewans/ulcer-instance:latest", "image@sha256:" + strings.Repeat("A", 64),
		"image;touch@sha256:" + strings.Repeat("a", 64), "https://registry.example/image@sha256:" + strings.Repeat("a", 64),
		"short-name/image@sha256:" + strings.Repeat("a", 64),
		"registry.example/../image@sha256:" + strings.Repeat("a", 64),
	} {
		if ValidImage(invalid) {
			t.Errorf("invalid image accepted: %q", invalid)
		}
	}
	if !Available(config.Host{PublicName: "host.example", PublicGRPC: "host.example:8443", InstanceImage: image}) {
		t.Fatal("valid SSH installation configuration is unavailable")
	}
	if Available(config.Host{PublicName: "host.example", PublicGRPC: "https://host.example:8443", InstanceImage: image}) {
		t.Fatal("invalid public gRPC endpoint was available")
	}

	environment, err := renderInstanceEnv(testID, "edge-01", "[2001:db8::1]:8443", "2001:db8::1")
	if err != nil {
		t.Fatal(err)
	}
	environmentText := string(environment)
	for _, expected := range []string{
		"ULCER_INSTANCE_ID=" + testID,
		"ULCER_INSTANCE_NAME=edge-01",
		"ULCER_HOST_GRPC=[2001:db8::1]:8443",
		"ULCER_HOST_SERVER_NAME=2001:db8::1",
	} {
		if !strings.Contains(environmentText, expected) {
			t.Errorf("environment is missing %q", expected)
		}
	}
	if _, err := renderInstanceEnv(testID, "unsafe name", "host:8443", "host"); err == nil {
		t.Fatal("unsafe name was rendered")
	}

	unit, err := renderUnit(testID, image)
	if err != nil {
		t.Fatal(err)
	}
	unitText := string(unit)
	for _, expected := range []string{
		"--read-only", "--security-opt=no-new-privileges", "--cap-drop=all", "--pids-limit=128",
		"--memory=256m", "--cpus=1", "--tmpfs=/var/lib/ulcer-instance", "Restart=on-failure", image,
	} {
		if !strings.Contains(unitText, expected) {
			t.Errorf("unit is missing %q", expected)
		}
	}
	for _, secret := range []string{"password", "passphrase", "PRIVATE KEY"} {
		if strings.Contains(unitText, secret) {
			t.Errorf("unit contains secret marker %q", secret)
		}
	}
}

func TestPreflightRequiresCurrentRuntimeImageArchitecture(t *testing.T) {
	for _, expected := range []string{"debian|ubuntu", "x86_64|amd64", "cgroup.controllers"} {
		if !strings.Contains(preflightCommand, expected) {
			t.Errorf("preflight command is missing %q", expected)
		}
	}
}

func newSigner(t *testing.T) ssh.Signer {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}
