package http

import (
	"path"

	"github.com/gofiber/fiber/v3"

	qrapi "github.com/rodrigomm/proyectot/qr-api"
	"github.com/rodrigomm/proyectot/qr-api/internal/adapters/inbound/http/middleware"
	"github.com/rodrigomm/proyectot/qr-api/internal/adapters/inbound/http/problem"
	"github.com/rodrigomm/proyectot/qr-api/internal/adapters/inbound/http/swaggerui"
)

// docsAssetTypes is the /docs allow-list: an explicit map makes path traversal impossible.
var docsAssetTypes = map[string]string{
	"index.html":                      fiber.MIMETextHTMLCharsetUTF8,
	"init.js":                         "text/javascript; charset=utf-8",
	"swagger-ui.css":                  "text/css; charset=utf-8",
	"swagger-ui-bundle.js":            "text/javascript; charset=utf-8",
	"swagger-ui-standalone-preset.js": "text/javascript; charset=utf-8",
	"favicon-32x32.png":               "image/png",
}

// openAPIHandler serves the contract this service implements.
func openAPIHandler(c fiber.Ctx) error {
	c.Set(fiber.HeaderContentType, "application/yaml; charset=utf-8")
	c.Set(fiber.HeaderCacheControl, "public, max-age=300")
	return c.Send(qrapi.OpenAPISpec)
}

// docsHandler serves the vendored Swagger UI, relaxing the CSP for this page only.
func docsHandler(c fiber.Ctx) error {
	name := path.Base(c.Params("*", "index.html"))
	if name == "." || name == "/" || name == "" {
		name = "index.html"
	}
	contentType, ok := docsAssetTypes[name]
	if !ok {
		return problem.NotFound()
	}
	content, err := swaggerui.Assets.ReadFile(name)
	if err != nil {
		return problem.NotFound()
	}

	c.Set(fiber.HeaderContentSecurityPolicy, middleware.DocsContentSecurityPolicy)
	c.Set(fiber.HeaderContentType, contentType)
	c.Set(fiber.HeaderCacheControl, "public, max-age=3600")
	return c.Send(content)
}
