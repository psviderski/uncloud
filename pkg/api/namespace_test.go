package api

import (
	"testing"
)

func TestValidateNamespaceName(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		wantErr   bool
	}{
		// Valid names
		{
			name:      "simple lowercase",
			namespace: "prod",
			wantErr:   false,
		},
		{
			name:      "with hyphens",
			namespace: "my-app-prod",
			wantErr:   false,
		},
		{
			name:      "with numbers",
			namespace: "app-v2",
			wantErr:   false,
		},
		{
			name:      "default",
			namespace: "default",
			wantErr:   false,
		},
		{
			name:      "single char",
			namespace: "a",
			wantErr:   false,
		},
		{
			name:      "numeric",
			namespace: "123",
			wantErr:   false,
		},
		{
			name:      "max length 63",
			namespace: "abcdefghij-abcdefghij-abcdefghij-abcdefghij-abcdefghij-abcdefg",
			wantErr:   false,
		},

		// Invalid names
		{
			name:      "empty",
			namespace: "",
			wantErr:   true,
		},
		{
			name:      "uppercase",
			namespace: "Prod",
			wantErr:   true,
		},
		{
			name:      "mixed case",
			namespace: "myApp",
			wantErr:   true,
		},
		{
			name:      "starts with hyphen",
			namespace: "-prod",
			wantErr:   true,
		},
		{
			name:      "ends with hyphen",
			namespace: "prod-",
			wantErr:   true,
		},
		{
			name:      "underscore",
			namespace: "my_app",
			wantErr:   true,
		},
		{
			name:      "dot",
			namespace: "my.app",
			wantErr:   true,
		},
		{
			name:      "space",
			namespace: "my app",
			wantErr:   true,
		},
		{
			name:      "too long",
			namespace: "abcdefghij-abcdefghij-abcdefghij-abcdefghij-abcdefghij-abcdefghi",
			wantErr:   true,
		},
		{
			name:      "special chars",
			namespace: "app@prod",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNamespaceName(tt.namespace)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNamespaceName(%q) error = %v, wantErr %v", tt.namespace, err, tt.wantErr)
			}
		})
	}
}

func TestDefaultNamespace(t *testing.T) {
	if DefaultNamespace != "default" {
		t.Errorf("DefaultNamespace = %q, want %q", DefaultNamespace, "default")
	}

	// Ensure default namespace is valid
	if err := ValidateNamespaceName(DefaultNamespace); err != nil {
		t.Errorf("DefaultNamespace %q is invalid: %v", DefaultNamespace, err)
	}
}
