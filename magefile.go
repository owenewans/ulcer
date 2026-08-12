//go:build mage

package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/magefile/mage/mg"
)

var Default = Check

var goPackages = []string{
	"./cmd/...",
	"./gen/...",
	"./internal/...",
	"./versions/...",
}

// Generate regenerates protobuf bindings.
func Generate() error {
	return run("", nil, "protoc",
		"--go_out=.",
		"--go_opt=module=github.com/owenewans/ulcer",
		"--go-grpc_out=.",
		"--go-grpc_opt=module=github.com/owenewans/ulcer",
		"api/proto/v1/control.proto",
	)
}

// Format formats all project Go packages.
func Format() error {
	if err := run("", nil, "gofmt", "-w", "magefile.go"); err != nil {
		return err
	}
	return run("", nil, "go", append([]string{"fmt"}, goPackages...)...)
}

// Test runs the Go unit tests.
func Test() error {
	return run("", nil, "go", append([]string{"test"}, goPackages...)...)
}

// Check runs static analysis, race tests and UI checks.
func Check() error {
	if err := Freeze(); err != nil {
		return err
	}
	if err := run("", nil, "go", append([]string{"vet"}, goPackages...)...); err != nil {
		return err
	}
	if err := run("", nil, "go", append([]string{"test", "-race"}, goPackages...)...); err != nil {
		return err
	}
	if err := run("web", nil, "bun", "run", "typecheck"); err != nil {
		return err
	}
	return run("web", nil, "bun", "run", "lint")
}

// Freeze verifies every submodule gitlink against the reviewed source manifest.
func Freeze() error {
	data, err := os.ReadFile(filepath.Join("versions", "sources.json"))
	if err != nil {
		return err
	}
	var manifest struct {
		Sources []struct {
			Path   string `json:"path"`
			Commit string `json:"commit"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("decode source freeze: %w", err)
	}
	command := exec.Command("git", "ls-files", "--stage")
	output, err := command.Output()
	if err != nil {
		return fmt.Errorf("read gitlinks: %w", err)
	}
	gitlinks := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 4 && fields[0] == "160000" {
			gitlinks[fields[3]] = fields[1]
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	for _, source := range manifest.Sources {
		if gitlinks[source.Path] != source.Commit {
			return fmt.Errorf("%s is %q, freeze requires %q", source.Path, gitlinks[source.Path], source.Commit)
		}
		delete(gitlinks, source.Path)
	}
	if len(gitlinks) != 0 {
		return fmt.Errorf("unreviewed gitlinks in index: %v", gitlinks)
	}
	return nil
}

// Build builds HOST, INSTANCE and the production UI.
func Build() error {
	mg.Deps(Go.Build, Web.Build)
	return nil
}

// E2e runs the browser and real mTLS agent test.
func E2e() error {
	environment := map[string]string{
		"HTTP_PROXY":  "",
		"HTTPS_PROXY": "",
		"ALL_PROXY":   "",
		"http_proxy":  "",
		"https_proxy": "",
		"all_proxy":   "",
		"NO_PROXY":    "localhost,127.0.0.1,::1",
		"no_proxy":    "localhost,127.0.0.1,::1",
	}
	return run("web", environment, "bun", "run", "test:e2e")
}

type Go mg.Namespace

// Build builds the HOST and INSTANCE binaries into bin/.
func (Go) Build() error {
	if err := os.MkdirAll("bin", 0o755); err != nil {
		return err
	}
	if err := run("", nil, "go", "build", "-trimpath", "-o", "bin/ulcer-host", "./cmd/ulcer-host"); err != nil {
		return err
	}
	return run("", nil, "go", "build", "-trimpath", "-o", "bin/ulcer-instance", "./cmd/ulcer-instance")
}

type Web mg.Namespace

// Install installs the frozen Bun dependency graph.
func (Web) Install() error {
	return run("web", nil, "bun", "install", "--frozen-lockfile")
}

// Dev starts the Next.js development server.
func (Web) Dev() error {
	return run("web", nil, "bun", "run", "dev")
}

// Build creates the standalone production UI.
func (Web) Build() error {
	return run("web", map[string]string{"NEXT_TELEMETRY_DISABLED": "1"}, "bun", "run", "build")
}

type Podman mg.Namespace

// Smoke builds the runtime images and checks HOST health in a locked-down container.
func (Podman) Smoke() error {
	return run("", nil, filepath.Join("hack", "podman-smoke.sh"))
}

type Release mg.Namespace

// Binaries builds reproducible HOST and INSTANCE binaries for GOOS/GOARCH.
func (Release) Binaries() error {
	goos := environmentOr("GOOS", "linux")
	goarch := environmentOr("GOARCH", "amd64")
	if err := os.MkdirAll("dist", 0o755); err != nil {
		return err
	}
	environment := map[string]string{"CGO_ENABLED": "0", "GOOS": goos, "GOARCH": goarch}
	for _, binary := range []string{"host", "instance"} {
		output := filepath.Join("dist", fmt.Sprintf("ulcer-%s-%s-%s", binary, goos, goarch))
		if err := run("", environment, "go", "build", "-trimpath", "-ldflags=-s -w", "-o", output, "./cmd/ulcer-"+binary); err != nil {
			return err
		}
	}
	return nil
}

// Checksums writes deterministic SHA-256 checksums for release files.
func (Release) Checksums() error {
	entries, err := os.ReadDir("dist")
	if err != nil {
		return err
	}
	output, err := os.Create(filepath.Join("dist", "SHA256SUMS"))
	if err != nil {
		return err
	}
	defer output.Close()
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "SHA256SUMS" {
			continue
		}
		path := filepath.Join("dist", entry.Name())
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if _, err := fmt.Fprintf(output, "%x  %s\n", hash.Sum(nil), entry.Name()); err != nil {
			return err
		}
	}
	return output.Sync()
}

func run(directory string, environment map[string]string, name string, arguments ...string) error {
	command := exec.Command(name, arguments...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Stdin = os.Stdin
	if directory != "" {
		command.Dir = directory
	}
	mergedEnvironment := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			mergedEnvironment[key] = value
		}
	}
	for key, value := range environment {
		mergedEnvironment[key] = value
	}
	command.Env = make([]string, 0, len(mergedEnvironment))
	for key, value := range mergedEnvironment {
		command.Env = append(command.Env, key+"="+value)
	}
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func environmentOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
