package versions

import _ "embed"

// EnginesJSON is the reviewed artifact catalog used by the API and release jobs.
//
//go:embed engines.json
var EnginesJSON []byte
