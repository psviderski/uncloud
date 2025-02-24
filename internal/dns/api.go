package dns

const (
	RecordTypeA    RecordType = "A"
	RecordTypeAAAA RecordType = "AAAA"
)

type RecordType string

type DomainResponse struct {
	Name  string `json:"name,omitempty"`
	Token string `json:"token,omitempty"`
}

type RecordRequest struct {
	Name   string     `json:"name,omitempty"`
	Type   RecordType `json:"type,omitempty"`
	Values []string   `json:"values,omitempty"`
}

type RecordResponse struct {
	RecordRequest
	FQDN string `json:"fqdn,omitempty"`
}

type AuthErrorResponse struct {
	Status  int           `json:"status,omitempty"`
	Message string        `json:"msg,omitempty"`
	Data    authErrorData `json:"data,omitempty"`
}

type authErrorData struct {
	NoDomain bool `json:"noDomain,omitempty"`
}
