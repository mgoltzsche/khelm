package helm

import (
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadGeneratorConfig(t *testing.T) {
	f, err := os.Open(filepath.Join(rootDir, "example/invalid-requirements-lock/generator.yaml"))
	require.NoError(t, err)
	defer f.Close()
	cfg, err := ReadGeneratorConfig(f)
	require.NoError(t, err)
	require.NotNil(t, cfg, "result")
}

func TestReadGeneratorConfigUnsupportedField(t *testing.T) {
	log.SetFlags(0)
	f, err := os.Open(filepath.Join(rootDir, "example/unsupported-field/generator.yaml"))
	require.NoError(t, err)
	defer f.Close()
	_, err = ReadGeneratorConfig(f)
	require.NoError(t, err)
}
