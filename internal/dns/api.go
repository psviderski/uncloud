package dns

type DomainResponse struct {
	Name  string `json:"name,omitempty"`
	Token string `json:"token,omitempty"`
}

type AuthErrorResponse struct {
	Status  int           `json:"status,omitempty"`
	Message string        `json:"msg,omitempty"`
	Data    authErrorData `json:"data,omitempty"`
}

type authErrorData struct {
	NoDomain bool `json:"noDomain,omitempty"`
}
