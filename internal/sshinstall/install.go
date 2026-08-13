package sshinstall

import (
	"bytes"
	"context"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/owenewans/ulcer/internal/config"
	"github.com/owenewans/ulcer/internal/control"
	"github.com/owenewans/ulcer/internal/events"
	"github.com/owenewans/ulcer/internal/model"
	"github.com/owenewans/ulcer/internal/pki"
	"github.com/owenewans/ulcer/internal/store"
	"golang.org/x/crypto/ssh"
)

const (
	installTimeout   = 10 * time.Minute
	onlineTimeout    = 90 * time.Second
	installSlots     = 2
	preflightCommand = `test "$(id -u)" = 0 && test -r /etc/os-release && . /etc/os-release && case "${ID:-}" in debian|ubuntu) ;; *) exit 20 ;; esac && case "$(uname -m)" in x86_64|amd64) ;; *) exit 21 ;; esac && test -d /run/systemd/system && command -v systemctl >/dev/null && test -r /sys/fs/cgroup/cgroup.controllers`
	packagesCommand  = `export DEBIAN_FRONTEND=noninteractive; apt-get update && apt-get install --yes --no-install-recommends aardvark-dns ca-certificates containernetworking-plugins fuse-overlayfs iproute2 nftables podman procps slirp4netns uidmap`
)

var (
	ErrUnavailable = errors.New("SSH installation is unavailable")
	ErrBusy        = errors.New("too many SSH installations are in progress")
	ErrInvalidAuth = errors.New("exactly one valid SSH authentication method is required")
	ErrInvalidUser = errors.New("SSH installation currently requires the root user")
	ErrConnect     = errors.New("SSH connection or authentication failed")
	ErrPreflight   = errors.New("remote SSH preflight failed")
	ErrInstall     = errors.New("remote SSH installation failed")
	ErrOnline      = errors.New("instance did not connect before the deadline")
)

type Request struct {
	Name               string
	Host               string
	Port               int
	User               string
	HostKeySHA256      string
	Password           []byte
	PrivateKeyPEM      []byte
	PrivateKeyPassword []byte
}

type Installer struct {
	store     *store.Store
	authority *pki.Authority
	events    *events.Bus
	hub       *control.Hub
	config    config.Host
	slots     chan struct{}
}

func New(database *store.Store, authority *pki.Authority, eventBus *events.Bus, hub *control.Hub, configuration config.Host) *Installer {
	return &Installer{
		store: database, authority: authority, events: eventBus, hub: hub, config: configuration,
		slots: make(chan struct{}, installSlots),
	}
}

func (i *Installer) Install(ctx context.Context, request Request) (instance model.Instance, returnedErr error) {
	defer wipe(request.Password)
	defer wipe(request.PrivateKeyPEM)
	defer wipe(request.PrivateKeyPassword)
	if !Available(i.config) {
		return model.Instance{}, ErrUnavailable
	}
	if err := model.ValidateInstanceName(request.Name); err != nil {
		return model.Instance{}, err
	}
	if request.User != "root" {
		return model.Instance{}, ErrInvalidUser
	}
	callback, err := fingerprintCallback(request.HostKeySHA256)
	if err != nil {
		return model.Instance{}, err
	}
	select {
	case i.slots <- struct{}{}:
		defer func() { <-i.slots }()
	default:
		return model.Instance{}, ErrBusy
	}
	authMethod, err := authenticationMethod(request)
	if err != nil {
		return model.Instance{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, installTimeout)
	defer cancel()
	resolveContext, resolveCancel := context.WithTimeout(ctx, networkTimeout)
	target, err := resolveTarget(resolveContext, request.Host, request.Port)
	resolveCancel()
	if err != nil {
		return model.Instance{}, err
	}
	client, closeClient, err := connect(ctx, target, request.User, callback, authMethod)
	if err != nil {
		if errors.Is(err, ErrHostKeyMismatch) {
			return model.Instance{}, ErrHostKeyMismatch
		}
		if ctx.Err() != nil {
			return model.Instance{}, ErrInstall
		}
		return model.Instance{}, ErrConnect
	}
	defer closeClient()
	if err := runCommand(client, preflightCommand); err != nil {
		return model.Instance{}, ErrPreflight
	}

	id := uuid.NewString()
	environment, err := renderInstanceEnv(id, request.Name, i.config.PublicGRPC, i.config.PublicName)
	if err != nil {
		return model.Instance{}, ErrUnavailable
	}
	defer wipe(environment)
	unit, err := renderUnit(id, i.config.InstanceImage)
	if err != nil {
		return model.Instance{}, ErrUnavailable
	}
	defer wipe(unit)
	credentials, err := i.authority.IssueInstanceFiles(id)
	if err != nil {
		return model.Instance{}, ErrInstall
	}
	defer wipe(credentials.Certificate)
	defer wipe(credentials.PrivateKey)
	defer wipe(credentials.CA)

	instance = model.Instance{
		ID: id, Name: request.Name, CreatedAt: time.Now().UTC(), Capabilities: []string{}, Phase: "enrolled",
	}
	if err := i.store.PutInstance(instance); err != nil {
		return model.Instance{}, ErrInstall
	}
	enrolled := true
	remoteTouched := false
	defer func() {
		if returnedErr == nil || !enrolled {
			return
		}
		release := i.hub.Revoke(id)
		defer release()
		if remoteTouched {
			if err := cleanupRemote(client, id); err != nil {
				cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
				cleanupClient, closeCleanup, connectErr := connect(cleanupContext, target, request.User, callback, authMethod)
				if connectErr == nil {
					_ = cleanupRemote(cleanupClient, id)
					closeCleanup()
				}
				cleanupCancel()
			}
		}
		i.publish("instance.ssh_install.failed", id, map[string]any{"name": request.Name})
		_, deleteErr := i.store.DeleteInstance(id)
		if deleteErr == nil {
			i.publish("instance.deleted", id, map[string]any{"name": request.Name, "rollback": true})
		}
	}()
	i.publish("instance.enrolled", id, map[string]any{"name": request.Name, "method": "ssh"})
	i.publish("instance.ssh_install.started", id, map[string]any{"name": request.Name})

	if err := runCommand(client, packagesCommand); err != nil {
		return model.Instance{}, ErrInstall
	}
	remoteTouched = true
	if err := prepareRemote(client, id); err != nil {
		return model.Instance{}, ErrInstall
	}
	directory := "/etc/ulcer/instances/" + id
	files := []struct {
		path    string
		mode    string
		owner   string
		content []byte
	}{
		{directory + "/instance.env", "0600", "root:root", environment},
		{directory + "/instance.crt", "0640", "root:65532", credentials.Certificate},
		{directory + "/instance.key", "0640", "root:65532", credentials.PrivateKey},
		{directory + "/ca.crt", "0640", "root:65532", credentials.CA},
		{"/etc/systemd/system/ulcer-instance-" + id + ".service", "0644", "root:root", unit},
	}
	for _, file := range files {
		if err := uploadFile(client, file.path, file.mode, file.owner, file.content); err != nil {
			return model.Instance{}, ErrInstall
		}
	}
	if err := runCommand(client, "podman pull --quiet "+i.config.InstanceImage); err != nil {
		return model.Instance{}, ErrInstall
	}
	if err := runCommand(client, "systemctl daemon-reload && systemctl enable --now ulcer-instance-"+id+".service"); err != nil {
		return model.Instance{}, ErrInstall
	}
	if err := i.waitOnline(ctx, id); err != nil {
		return model.Instance{}, err
	}
	if ctx.Err() != nil {
		return model.Instance{}, ErrOnline
	}
	succeeded := false
	i.hub.IfOnline(id, func() {
		instance, err = i.store.Instance(id)
		if err != nil {
			return
		}
		enrolled = false
		i.publish("instance.ssh_install.succeeded", id, map[string]any{"name": request.Name})
		succeeded = true
	})
	if !succeeded {
		return model.Instance{}, ErrInstall
	}
	return instance, nil
}

func authenticationMethod(request Request) (ssh.AuthMethod, error) {
	password := len(request.Password) > 0
	privateKey := len(request.PrivateKeyPEM) > 0
	if password == privateKey || (len(request.PrivateKeyPassword) > 0 && !privateKey) {
		return nil, ErrInvalidAuth
	}
	if password {
		return ssh.PasswordCallback(func() (string, error) {
			return string(request.Password), nil
		}), nil
	}
	var signer ssh.Signer
	var err error
	if len(request.PrivateKeyPassword) > 0 {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(request.PrivateKeyPEM, request.PrivateKeyPassword)
	} else {
		signer, err = ssh.ParsePrivateKey(request.PrivateKeyPEM)
	}
	if err != nil {
		return nil, ErrInvalidAuth
	}
	return ssh.PublicKeys(signer), nil
}

func connect(ctx context.Context, target resolvedTarget, user string, callback ssh.HostKeyCallback, authMethod ssh.AuthMethod) (*ssh.Client, func(), error) {
	connection, err := dialTarget(ctx, target)
	if err != nil {
		return nil, func() {}, err
	}
	stopCancellation := context.AfterFunc(ctx, func() { _ = connection.Close() })
	clientConnection, channels, requests, err := ssh.NewClientConn(connection, target.host, &ssh.ClientConfig{
		User: user, Auth: []ssh.AuthMethod{authMethod}, HostKeyCallback: callback,
	})
	if err != nil {
		stopCancellation()
		connection.Close()
		return nil, func() {}, err
	}
	if err := connection.SetDeadline(time.Time{}); err != nil {
		stopCancellation()
		clientConnection.Close()
		return nil, func() {}, err
	}
	client := ssh.NewClient(clientConnection, channels, requests)
	return client, func() {
		stopCancellation()
		_ = client.Close()
	}, nil
}

func runCommand(client *ssh.Client, command string) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	session.Stdout = io.Discard
	session.Stderr = io.Discard
	return session.Run(command)
}

func prepareRemote(client *ssh.Client, id string) error {
	if !safeUUID(id) {
		return ErrInstall
	}
	return runCommand(client, "install -d -m 0750 -o root -g 65532 /etc/ulcer/instances/"+id)
}

func uploadFile(client *ssh.Client, path, mode, owner string, content []byte) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	session.Stdin = bytes.NewReader(content)
	session.Stdout = io.Discard
	session.Stderr = io.Discard
	return session.Run("umask 077; cat > " + path + " && chown " + owner + " " + path + " && chmod " + mode + " " + path)
}

func cleanupRemote(client *ssh.Client, id string) error {
	if !safeUUID(id) {
		return ErrInstall
	}
	unit := "ulcer-instance-" + id + ".service"
	command := "systemctl disable --now " + unit + " >/dev/null 2>&1 || true; " +
		"podman rm --force ulcer-instance-" + id + " >/dev/null 2>&1 || true; " +
		"rm -f /etc/systemd/system/" + unit + "; rm -rf /etc/ulcer/instances/" + id + "; systemctl daemon-reload"
	return runCommand(client, command)
}

func (i *Installer) waitOnline(ctx context.Context, id string) error {
	timer := time.NewTimer(onlineTimeout)
	defer timer.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ErrOnline
		default:
		}
		if i.hub.Online(id) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ErrOnline
		case <-timer.C:
			return ErrOnline
		case <-ticker.C:
		}
	}
}

func (i *Installer) publish(eventType, id string, data map[string]any) {
	_, _ = i.events.Publish(model.Event{Type: eventType, ResourceID: id, Data: data})
}

func wipe(data []byte) {
	for index := range data {
		data[index] = 0
	}
}
