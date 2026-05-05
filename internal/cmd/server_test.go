package cmd

import (
	"strings"
	"testing"

	"github.com/YaHeii/agentGo/internal/utils"
)

func TestValidateProviderConfigRequiresAPIKeyAndModel(t *testing.T) {
	t.Parallel()

	_, err := ProviderConfigFromAppConfig(utils.Config{})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "API_KEY") {
		t.Fatalf("expected API_KEY error, got %v", err)
	}

	_, err = ProviderConfigFromAppConfig(utils.Config{APIKey: "test-key"})
	if err == nil {
		t.Fatal("expected missing MODEL error")
	}
	if !strings.Contains(err.Error(), "MODEL") {
		t.Fatalf("expected MODEL error, got %v", err)
	}
}
