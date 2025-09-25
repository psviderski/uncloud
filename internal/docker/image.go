package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	dockercommand "github.com/docker/cli/cli/command"
	dockerconfig "github.com/docker/cli/cli/config"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/pkg/jsonmessage"
)

type PullImageMessage struct {
	Message jsonmessage.JSONMessage
	Err     error
}

// PullImage pulls a Docker image and returns a channel to receive progress messages.
func (cli *Client) PullImage(
	ctx context.Context, image string, opts image.PullOptions,
) (<-chan PullImageMessage, error) {
	if opts.RegistryAuth == "" {
		// Try to retrieve the authentication token for the image from the default local Docker config file.
		if encodedAuth, err := RetrieveLocalDockerRegistryAuth(image); err == nil {
			opts.RegistryAuth = encodedAuth
		}
	}

	respBody, err := cli.ImagePull(ctx, image, opts)
	if err != nil {
		return nil, err
	}
	defer respBody.Close()

	decoder := json.NewDecoder(respBody)
	ch := make(chan PullImageMessage)

	go func() {
		defer close(ch)
		var jm jsonmessage.JSONMessage

		for {
			if err = decoder.Decode(&jm); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				ch <- PullImageMessage{Err: fmt.Errorf("decode image pull message: %w", err)}
				break
			}

			msg := PullImageMessage{Message: jm}
			if jm.Error != nil {
				msg.Err = errors.New(jm.Error.Message)
			}

			select {
			case err = <-ctx.Done():
				ch <- PullImageMessage{Err: ctx.Err()}
				return
			default:
				ch <- msg
			}
		}
	}()

	return ch, nil
}

// RetrieveLocalDockerRegistryAuth retrieves the authentication token for the specified image from the local Docker
// config file. It returns the encoded authentication token if it contains any credentials, or an empty string if
// no credentials are found.
func RetrieveLocalDockerRegistryAuth(image string) (string, error) {
	// Try to retrieve the authentication token for the image from the default local Docker config file.
	dockerConfig := dockerconfig.LoadDefaultConfigFile(os.Stderr)
	encodedAuth, err := dockercommand.RetrieveAuthTokenFromImage(dockerConfig, image)
	if err != nil {
		return "", err
	}
	// The encodedAuth can be a base64-encoded "{}" (empty JSON object) or include a server address but no credentials.
	// Return encodedAuth only if it contains any credentials.
	auth, err := registry.DecodeAuthConfig(encodedAuth)
	if err != nil {
		return "", fmt.Errorf("decode auth config: %w", err)
	}

	if auth.Username == "" &&
		auth.Password == "" &&
		auth.Auth == "" &&
		auth.IdentityToken == "" &&
		auth.RegistryToken == "" {
		return "", nil
	}

	return encodedAuth, nil
}
