// Package swaggerui embeds a pinned swagger-ui-dist so /docs needs no CDN and keeps CSP 'self'.
package swaggerui

import "embed"

// Assets holds index.html, init.js and the swagger-ui-dist files.
//
//go:embed index.html init.js swagger-ui.css swagger-ui-bundle.js swagger-ui-standalone-preset.js favicon-32x32.png
var Assets embed.FS

// Version is the pinned swagger-ui-dist release.
//
//go:embed VERSION
var Version string
