package api

// ClusterDomain is the domain name assigned to the cluster.
type ClusterDomain struct {
	Name string
	// Reserved reports whether the domain is reserved in Uncloud DNS or set and managed externally.
	Reserved bool
}
