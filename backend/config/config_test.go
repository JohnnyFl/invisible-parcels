package config

import (
	"slices"
	"testing"
)

func TestDefaultSecrets_ListsEveryDefault(t *testing.T) {
	c := Config{AppSecret: "x", IngestSecret: "dev-ingest-secret", AdminPassword: "y", SeniorPassword: "senior", OperatorPassword: "z"}
	got := DefaultSecrets(c)
	if !slices.Equal(got, []string{"INGEST_SECRET", "SENIOR_PASSWORD"}) {
		t.Fatal(got)
	}
}
