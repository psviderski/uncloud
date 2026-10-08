package api

// StoreVersion records the highest processed database version for each Corrosion actor UUID.
// A version can include changes that were superseded, so it does not guarantee that all data
// through that version is available locally.
type StoreVersion map[string]uint64

// MergeMax keeps the highest observed version for each actor in other.
// The receiver must be initialised before calling MergeMax.
func (v StoreVersion) MergeMax(other StoreVersion) {
	for actor, version := range other {
		v[actor] = max(v[actor], version)
	}
}
