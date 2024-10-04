package corroservice

import "context"

type Service interface {
	Start(ctx context.Context) error
	Restart(ctx context.Context) error
	Running() bool
}
