package compose

const RegistryExtensionKey = "x-registry"

type RegistrySource map[string]struct {
	Username string
	Password string
}
