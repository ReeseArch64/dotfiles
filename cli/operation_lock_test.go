package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOperationLockSerializesWithoutCreatingLocalFiles(t *testing.T) {
	home := t.TempDir()
	key := filepath.Join(home, "operation")
	unlock, err := lockOperation(key, "teste")
	if err != nil {
		t.Fatal(err)
	}
	if second, err := lockOperation(key, "teste"); err == nil {
		second()
		unlock()
		t.Fatal("duas operações adquiriram o mesmo lock")
	}
	unlock()
	unlock, err = lockOperation(key, "teste")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Fatalf("arquivos criados na pasta de trabalho: %v, %v", entries, err)
	}
}
