//go:build windows

package main

import "testing"

type fakeAutostartRegistryKey struct {
	closed  bool
	deleted string
}

func (k *fakeAutostartRegistryKey) Close() error                        { k.closed = true; return nil }
func (k *fakeAutostartRegistryKey) DeleteValue(name string) error       { k.deleted = name; return nil }
func (k *fakeAutostartRegistryKey) SetStringValue(string, string) error { return nil }

func TestSetAutostartCreatesOrOpensRunKey(t *testing.T) {
	key := &fakeAutostartRegistryKey{}
	called := false
	previous := openOrCreateAutostartKey
	openOrCreateAutostartKey = func() (autostartRegistryKey, error) {
		called = true
		return key, nil
	}
	t.Cleanup(func() { openOrCreateAutostartKey = previous })

	if err := setAutostart(false); err != nil {
		t.Fatalf("setAutostart(false) failed: %v", err)
	}
	if !called {
		t.Fatal("setAutostart did not open or create the per-user Run key")
	}
	if key.deleted != autostartValueName {
		t.Fatalf("deleted value = %q, want %q", key.deleted, autostartValueName)
	}
	if !key.closed {
		t.Fatal("Run key was not closed")
	}
}
