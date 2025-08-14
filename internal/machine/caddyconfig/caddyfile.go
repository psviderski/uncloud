package caddyconfig

import (
	"fmt"

	"github.com/psviderski/uncloud/pkg/api"
)

func GenerateCaddyfile(containers []api.ServiceContainer, verifyResponse string) (string, error) {
	return fmt.Sprintf(`http:// {
	handle %s {
		respond "%s" 200
	}
	log
}
`, VerifyPath, verifyResponse), nil
}
