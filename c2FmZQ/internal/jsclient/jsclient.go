package jsclient

import "embed"

//go:embed *.html *.css *.js *.png *.map
var FS embed.FS
