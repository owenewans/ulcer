package buildinfo

import (
	"strings"
	"testing"
)

func TestCurrentUsesInjectedRevisionInLicenseURL(t *testing.T) {
	oldVersion, oldRevision, oldSourceRef := Version, Revision, SourceRef
	t.Cleanup(func() { Version, Revision, SourceRef = oldVersion, oldRevision, oldSourceRef })
	Version = "v1.2.3"
	Revision = "feature/ref"
	SourceRef = "v1.2.3"

	info := Current()
	if info.Version != Version || info.Revision != Revision || info.SourceRef != SourceRef {
		t.Fatalf("unexpected build info: %+v", info)
	}
	if info.SourceURL != SourceURL {
		t.Fatalf("source URL = %q", info.SourceURL)
	}
	if !strings.Contains(info.LicenseURL, "/blob/feature%2Fref/LICENSE") {
		t.Fatalf("license URL did not safely use revision: %q", info.LicenseURL)
	}
	if strings.Contains(info.LicenseURL, "master") {
		t.Fatalf("license URL used a hardcoded branch: %q", info.LicenseURL)
	}
}

func TestCurrentRejectsUnsafeInjectedValues(t *testing.T) {
	oldVersion, oldRevision, oldSourceRef := Version, Revision, SourceRef
	t.Cleanup(func() { Version, Revision, SourceRef = oldVersion, oldRevision, oldSourceRef })
	Version = "bad\nversion"
	Revision = "bad\rrevision"
	SourceRef = "bad\x00ref"
	info := Current()
	if strings.ContainsAny(info.Version+info.Revision+info.SourceRef, "\r\n\x00") {
		t.Fatalf("unsafe metadata survived sanitization: %+v", info)
	}
}

func TestLicenseURLCannotContainTraversalSegment(t *testing.T) {
	oldVersion, oldRevision, oldSourceRef := Version, Revision, SourceRef
	t.Cleanup(func() { Version, Revision, SourceRef = oldVersion, oldRevision, oldSourceRef })
	Version = "development"
	Revision = ".."
	SourceRef = ".."
	if licenseURL := Current().LicenseURL; strings.Contains(licenseURL, "/../") {
		t.Fatalf("license URL contains traversal: %q", licenseURL)
	}
}

func TestSourceRevisionURLUsesImmutableRevision(t *testing.T) {
	if got := SourceRevisionURL("feature/ref"); got != SourceURL+"/tree/feature%2Fref" {
		t.Fatalf("source revision URL = %q", got)
	}
	if got := SourceRevisionURL("unknown"); got != SourceURL {
		t.Fatalf("unknown source URL = %q", got)
	}
}
