// Package postgresimage provides the build definition for Orca's pgBackRest-enabled PostgreSQL image.
package postgresimage

import _ "embed"

// dockerfile is kept beside the source Dockerfile so local and agent builds use the same definition.
//
//go:embed Dockerfile
var dockerfile []byte

// Dockerfile returns the pgBackRest-enabled PostgreSQL Dockerfile.
func Dockerfile() []byte {
	return dockerfile
}
