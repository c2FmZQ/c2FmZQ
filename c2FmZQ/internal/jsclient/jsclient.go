package jsclient

import "embed"

//go:embed *.html *.css *.js *.png thirdparty/*
var FS embed.FS
