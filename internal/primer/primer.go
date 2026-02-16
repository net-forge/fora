package primer

import _ "embed"

//go:embed primer.md
var content string

func Content() string {
	return content
}
