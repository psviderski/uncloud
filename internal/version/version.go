package version

var version string

func String() string {
	if version == "" {
		return "999.0.0-dev"
	}
	return version
}
