package jsonrpc

// Middleware wraps a method handler with cross-cutting behavior (metrics,
// tracing, auth, result rewriting). It is a composition-time factory
// receiving the method name and the next handler in the chain; the RPCMethod
// it returns runs on every call and may act before and after next,
// short-circuit, or rewrite the result.
//
// The factory re-runs once per registered method on EVERY registry change —
// each Use, RegisterMethod, or RegisterTyped call recomposes the whole
// dispatch map — so it must be idempotent and side-effect free (create
// metrics collectors and similar one-time resources outside the factory and
// only reference them inside). A panic in the factory propagates to the
// Use/Register call that triggered recomposition, not to requests.
type Middleware func(methodName string, next RPCMethod) RPCMethod

// Use appends a global middleware. Middleware applies to every method,
// including methods registered later. The first middleware registered is the
// outermost: for Use(a); Use(b) the call order is a → b → handler.
//
// Composition happens copy-on-write at registration, so middleware adds zero
// per-request overhead beyond the wrapped handlers themselves. Middleware
// runs with full process privileges and sees the raw params of every
// request before any validation — register only trusted code.
func (j *JSONRPC) Use(mw Middleware) {
	_ = j.updateRegistry(func(c *config) error {
		c.middlewares = append(c.middlewares, mw)
		return nil
	})
}

// compose rebuilds the dispatch map by wrapping every registered method in
// the middleware chain. With no middleware the raw registry is used as-is.
func (c *config) compose() {
	if len(c.middlewares) == 0 {
		c.composed = c.methods
		return
	}
	c.composed = make(RPCMethods, len(c.methods))
	for name, method := range c.methods {
		h := method
		for i := len(c.middlewares) - 1; i >= 0; i-- {
			h = c.middlewares[i](name, h)
		}
		c.composed[name] = h
	}
}
