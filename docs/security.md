# security model

## operator access

Ulcer has one operator identity rather than an internal role hierarchy. Initial
enrolment requires the local setup token and a valid TOTP confirmation. Login
creates a random opaque session; only its hash is persisted. Recovery codes are
single-use and shown once.

The setup token and generated CA private key are local credentials. Files are
created with mode `0600`. Production deployments should put the data directory
on encrypted storage and back it up offline.

## machine identity

HOST and INSTANCE communicate through TLS 1.3 with mutually verified
certificates. An INSTANCE certificate carries its exact identity in a SPIFFE URI
SAN. Machine certificates never authorize REST or UI administration.

Remote SSH credentials are bootstrap-only. A future enrolment flow installs a
one-use token, obtains a scoped certificate, verifies the connection and then
removes the bootstrap material.

## artifact trust

Instances run only Ulcer release artifacts whose digest, signature, provenance
and SBOM match the frozen manifest. GitHub credentials stay in CI or HOST secret
storage and are never injected into engine containers.

## known boundaries

- Badger supports one active HOST process; it is not a shared HA database.
- Generic eBPF accounting is not per-user accounting for shared encrypted listeners.
- A modified APK must be rebuilt or repackaged and signed again. Android package
  contents cannot remain mutable while retaining the original signature.
- Protocol engines retain their own licenses. A submodule or sidecar does not
  erase source-offer, notice or copyleft obligations.
