package buildinfo

import (
	"net/url"
	"runtime/debug"
	"strings"
	"unicode"
)

const SourceURL = "https://github.com/owenewans/ulcer"

var (
	Version   string
	Revision  string
	SourceRef string
)

type Info struct {
	Version    string `json:"version"`
	Revision   string `json:"revision"`
	SourceRef  string `json:"source_ref"`
	SourceURL  string `json:"source_url"`
	LicenseURL string `json:"license_url"`
}

func SourceRevisionURL(revision string) string {
	revision = clean(revision)
	if revision == "" || revision == "unknown" {
		return SourceURL
	}
	return SourceURL + "/tree/" + escapePathSegment(revision)
}

func Current() Info {
	version := clean(Version)
	revision := clean(Revision)
	sourceRef := clean(SourceRef)
	if runtimeInfo, ok := debug.ReadBuildInfo(); ok {
		if version == "" && runtimeInfo.Main.Version != "" && runtimeInfo.Main.Version != "(devel)" {
			version = clean(runtimeInfo.Main.Version)
		}
		if revision == "" {
			for _, setting := range runtimeInfo.Settings {
				if setting.Key == "vcs.revision" {
					revision = clean(setting.Value)
					break
				}
			}
		}
	}
	if version == "" {
		version = "development"
	}
	if revision == "" {
		revision = "unknown"
	}
	if sourceRef == "" {
		sourceRef = revision
	}
	if sourceRef == "" {
		sourceRef = "unknown"
	}
	licenseRef := revision
	if licenseRef == "unknown" {
		licenseRef = sourceRef
	}
	return Info{
		Version:    version,
		Revision:   revision,
		SourceRef:  sourceRef,
		SourceURL:  SourceURL,
		LicenseURL: SourceURL + "/blob/" + escapePathSegment(licenseRef) + "/LICENSE",
	}
}

func escapePathSegment(value string) string {
	escaped := url.PathEscape(value)
	if escaped == "." || escaped == ".." {
		escaped = strings.ReplaceAll(escaped, ".", "%2E")
	}
	return escaped
}

func clean(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 256 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return ""
	}
	return value
}
