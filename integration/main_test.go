package integration_tests

import (
	"os"
	"testing"

	"github.com/speakeasy-api/speakeasy/cmd"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

var rootCmd *cobra.Command

// Entrypoint for CLI integration tests
func TestMain(m *testing.M) {
	// Create a temporary directory
	if _, err := os.Stat(tempDir); err == nil {
		if err := os.RemoveAll(tempDir); err != nil {
			panic(err)
		}
	}

	if err := os.Mkdir(tempDir, 0o755); err != nil {
		panic(err)
	}

	// Defer the removal of the temp directory
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			panic(err)
		}
	}()

	rootCmd = cmd.CmdForTest(version, artifactArch)

	code := m.Run()
	os.Exit(code)
}

func setupTestDir(t *testing.T) string {
	workingDir, err := os.Getwd()
	assert.NoError(t, err)
	temp, err := createTempDir()
	assert.NoError(t, err)
	registerCleanup(t, workingDir, temp)

	return temp
}

func registerCleanup(t *testing.T, workingDir string, temp string) {
	t.Cleanup(func() {
		os.Chdir(workingDir)
		os.RemoveAll(temp)
	})
}
