package main

import "testing"

func TestFindSchematicName(t *testing.T) {
	entries := []talosSchematicEntry{
		{Name: "VMware"},
		{Name: "Prod-Cluster"},
	}

	if got, ok := findSchematicName(entries, "vmware"); !ok || got != "VMware" {
		t.Fatalf("expected case-insensitive match for VMware, got ok=%v name=%q", ok, got)
	}

	if _, ok := findSchematicName(entries, "dev-cluster"); ok {
		t.Fatal("expected no match for unknown schematic")
	}
}
