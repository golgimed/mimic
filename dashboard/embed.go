// Package dashboard embeds the static dashboard HTML into the compiled binary
// so it is served internally without needing a configurable on-disk path.
package dashboard

import _ "embed"

//go:embed index.html
var HTML string
