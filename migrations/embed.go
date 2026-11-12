// Package migrations embeds the SQL schema files so the server binary can
// apply them on start-up without shipping a separate folder.
package migrations

import "embed"

// FS holds every *.sql file in this directory, applied in filename order.
//
//go:embed *.sql
var FS embed.FS
