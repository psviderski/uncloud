package service

// displayNamespace returns a user-friendly namespace string.
// Empty namespace means mixed/unknown across containers.
func displayNamespace(ns string) string {
	if ns == "" {
		return "(mixed)"
	}
	return ns
}
