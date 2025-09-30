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

type PullPushImageMessage struct {
	Message jsonmessage.JSONMessage
	Err     error
}

// PullImage pulls an image and returns a channel to receive progress messages.
func (cli *Client) PullImage(
	ctx context.Context, image string, opts image.PullOptions,
) (<-chan PullPushImageMessage, error) {
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

	decoder := json.NewDecoder(respBody)
	ch := make(chan PullPushImageMessage)

	go func() {
		defer respBody.Close()
		defer close(ch)

		for {
			var jm jsonmessage.JSONMessage
			if err = decoder.Decode(&jm); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				ch <- PullPushImageMessage{Err: fmt.Errorf("decode image pull message: %w", err)}
				break
			}

			msg := PullPushImageMessage{Message: jm}
			if jm.Error != nil {
				msg.Err = errors.New(jm.Error.Message)
			}

			select {
			case <-ctx.Done():
				ch <- PullPushImageMessage{Err: ctx.Err()}
				return
			default:
				ch <- msg
			}
		}
	}()

	return ch, nil
}

// PushImage pushes an image and returns a channel to receive progress messages.
func (cli *Client) PushImage(
	ctx context.Context, image string, opts image.PushOptions,
) (<-chan PullPushImageMessage, error) {
	if opts.RegistryAuth == "" {
		// Try to retrieve the authentication token for the image from the default local Docker config file.
		if encodedAuth, err := RetrieveLocalDockerRegistryAuth(image); err == nil {
			opts.RegistryAuth = encodedAuth
		}
	}

	respBody, err := cli.ImagePush(ctx, image, opts)
	if err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(respBody)
	ch := make(chan PullPushImageMessage)

	go func() {
		defer respBody.Close()
		defer close(ch)

		for {
			var jm jsonmessage.JSONMessage
			if err = decoder.Decode(&jm); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				ch <- PullPushImageMessage{Err: fmt.Errorf("decode image push message: %w", err)}
				break
			}

			msg := PullPushImageMessage{Message: jm}
			if jm.Error != nil {
				msg.Err = errors.New(jm.Error.Message)
			}

			select {
			case <-ctx.Done():
				ch <- PullPushImageMessage{Err: ctx.Err()}
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
