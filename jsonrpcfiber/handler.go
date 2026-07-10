// Package jsonrpcfiber adapts a jsonrpc.JSONRPC dispatcher to a Fiber v2
// (github.com/gofiber/fiber/v2) route handler.
//
// It mirrors the net/http adapter (jsonrpchttp): a present non-JSON
// Content-Type is rejected with 415, notifications produce 204 No Content,
// and everything that parses as a JSON-RPC message — including a -32700 for
// malformed JSON — is answered with 200 and a JSON-RPC body. The request body
// is bounded by Fiber's own BodyLimit (fiber.Config.BodyLimit), so cap it
// there; oversized bodies are rejected by Fiber before this handler runs.
// Compressed bodies (a Content-Encoding other than identity) are rejected
// with 415: Fiber's BodyLimit caps the compressed size, so decompressing
// them here would risk a decompression bomb.
//
// The handler implements no authentication, CORS, or CSRF protection —
// those are application policy. A missing Content-Type is tolerated, so
// with cookie-based auth a cross-site request can reach the handler
// without a CORS preflight; use token auth or add CSRF protection.
//
// It is a separate Go module so the Fiber and fasthttp dependency trees stay
// out of the core module's go.mod.
package jsonrpcfiber

import (
	"mime"
	"strings"

	"github.com/gofiber/fiber/v2"

	jsonrpc "github.com/gumeniukcom/golang-jsonrpc2/v2"
)

// Handler returns a Fiber v2 handler serving JSON-RPC 2.0 over the route it
// is registered on (register it with app.Post so only POST reaches it):
//
//	app.Post("/rpc", jsonrpcfiber.Handler(rpc))
func Handler(rpc *jsonrpc.JSONRPC) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if ct := c.Get(fiber.HeaderContentType); ct != "" {
			if mediaType, _, err := mime.ParseMediaType(ct); err != nil || mediaType != fiber.MIMEApplicationJSON {
				return c.SendStatus(fiber.StatusUnsupportedMediaType)
			}
		}
		// Reject compressed bodies: Fiber decompresses c.Body() before the
		// handler and BodyLimit caps the COMPRESSED size, so a small gzip/br
		// payload could inflate to gigabytes (decompression bomb). We do not
		// decompress; a client wanting compression must handle the size bound
		// itself.
		if enc := c.Get(fiber.HeaderContentEncoding); enc != "" && !strings.EqualFold(enc, "identity") {
			return c.SendStatus(fiber.StatusUnsupportedMediaType)
		}

		// c.UserContext() is the request-scoped context (context.Background()
		// unless the app set one), safe to hold past the pooled Ctx — unless
		// the app set a UserContext derived from the Ctx itself. c.Body() is
		// valid only within the handler and consumed synchronously here.
		resp := rpc.HandleRPCJSONRawMessage(c.UserContext(), c.Body())
		if len(resp) == 0 {
			// Notification (or all-notification batch): no JSON-RPC reply.
			return c.SendStatus(fiber.StatusNoContent)
		}

		c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
		c.Set(fiber.HeaderCacheControl, "no-store")
		c.Set(fiber.HeaderXContentTypeOptions, "nosniff")
		return c.Status(fiber.StatusOK).Send(resp)
	}
}
