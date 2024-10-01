package corroservice

type Service interface {
	Start() error
	Restart() error
	Running() bool
}
