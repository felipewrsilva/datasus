package storage_test

import (
	"testing"

	"datasus/internal/storage"
)

func TestNormalizeAndValidatePolicyPath_WindowsLocal(t *testing.T) {
	got, err := storage.NormalizeAndValidatePolicyPath(`c:\\dados\\arquivos\`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `c:\dados\arquivos` {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeAndValidatePolicyPath_UNC(t *testing.T) {
	got, err := storage.NormalizeAndValidatePolicyPath(`//server/caminho`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `\\server\caminho` {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeAndValidatePolicyPath_InvalidLocal(t *testing.T) {
	_, err := storage.NormalizeAndValidatePolicyPath(`c:caminho`)
	if err == nil {
		t.Fatal("expected validation error")
	}
}
