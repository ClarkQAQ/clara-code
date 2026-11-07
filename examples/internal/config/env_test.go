package config

import (
	"os"
	"testing"
)

func TestEnvPrefersNewNameAndFallsBack(t *testing.T) {
	const current = "CLARA_CORE_TEST_SETTING"
	t.Setenv("CLARY_TEST_SETTING", "legacy")
	t.Setenv(current, "new")
	if got := Env("TEST_SETTING"); got != "new" {
		t.Fatalf("got %q with both variables set", got)
	}

	t.Setenv(current, "")
	if got := Env("TEST_SETTING"); got != "" {
		t.Fatalf("got %q with new variable explicitly empty", got)
	}

	if err := os.Unsetenv(current); err != nil {
		t.Fatal(err)
	}
	if got := Env("TEST_SETTING"); got != "legacy" {
		t.Fatalf("got %q with only the legacy variable set", got)
	}
}
