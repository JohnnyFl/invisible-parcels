package config

import (
	"slices"
	"testing"
)

func TestDefaultSecrets_ListsOnlyUnchangedSecrets(t *testing.T) {
	c := Config{AppSecret: "x", IngestSecret: "dev-ingest-secret", AdminPassword: "demo", SeniorPassword: "demo", OperatorPassword: "demo"}
	// role passwords are public on purpose ("demo"); only the two secrets matter
	if got := DefaultSecrets(c); !slices.Equal(got, []string{"INGEST_SECRET"}) {
		t.Fatal(got)
	}
}

func TestLoad_RolePasswordsDefaultToDemo(t *testing.T) {
	for _, k := range []string{"ADMIN_PASSWORD", "SENIOR_PASSWORD", "OPERATOR_PASSWORD"} {
		t.Setenv(k, "")
	}
	c := Load()
	if c.AdminPassword != "demo" || c.SeniorPassword != "demo" || c.OperatorPassword != "demo" {
		t.Fatalf("%q %q %q", c.AdminPassword, c.SeniorPassword, c.OperatorPassword)
	}
}
