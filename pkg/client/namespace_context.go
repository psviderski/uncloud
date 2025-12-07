package client

import "context"

type namespaceContextKey struct{}

// WithNamespace stores the namespace in context for service resolution.
func WithNamespace(ctx context.Context, namespace string) context.Context {
	return context.WithValue(ctx, namespaceContextKey{}, namespace)
}

// namespaceFromContext retrieves the namespace set by WithNamespace, if any.
func namespaceFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if ns, ok := ctx.Value(namespaceContextKey{}).(string); ok {
		return ns
	}
	return ""
}
