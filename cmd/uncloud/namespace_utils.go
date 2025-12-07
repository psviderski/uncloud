package main

import "github.com/psviderski/uncloud/pkg/api"

// namespaceFromLabels returns the namespace label or default.
func namespaceFromLabels(labels map[string]string) string {
	ns := labels[api.LabelNamespace]
	if ns == "" {
		return api.DefaultNamespace
	}
	return ns
}

// displayNamespace returns a user-friendly namespace string.
// Empty namespace means mixed/unknown across containers.
func displayNamespace(ns string) string {
	if ns == "" {
		return "(mixed)"
	}
	return ns
}
