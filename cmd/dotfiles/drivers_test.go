package main

import (
	"slices"
	"testing"
)

func TestDriversIncludeRequestedPackages(t *testing.T) {
	want := []string{"base-devel", "vulkan-tools", "mesa-utils", "linux-headers"}
	if !slices.Equal(driverPackages, want) {
		t.Fatalf("pacotes inesperados: %v", driverPackages)
	}
}
