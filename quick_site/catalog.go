// Package quicksite holds the catalog shared by the launcher and website.
package quicksite

import _ "embed"

// Catalog is the versioned catalog bundled at build time.
//
//go:embed tools.json
var Catalog []byte
