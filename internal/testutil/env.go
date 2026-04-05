// Package testutil provides utility functions for testing.
package testutil

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// SetEnvForTest sets an environment variable for the duration of a test.
// The original value is restored when the test completes.
func SetEnvForTest(t *testing.T, key, value string) {
	t.Helper()
	oldValue, wasSet := os.LookupEnv(key)
	t.Cleanup(func() {
		var err error
		if wasSet {
			err = os.Setenv(key, oldValue)
		} else {
			err = os.Unsetenv(key)
		}
		require.NoError(t, err, "Failed to restore environment variable %s", key)
	})
	require.NoError(t, os.Setenv(key, value), "Failed to set environment variable %s", key)
}
