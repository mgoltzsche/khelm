package cmd

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestKustomizePlugin(t *testing.T) {
	file := filepath.Join("..", "..", "example", "exclude", "chartref.yaml")
	kustomizeGenCfg, err := ioutil.ReadFile(file)
	require.NoError(t, err)
	os.Setenv("KUSTOMIZE_PLUGIN_CONFIG_STRING", string(kustomizeGenCfg))
	os.Setenv("HELMR_ALLOW_UNKNOWN_REPOSITORIES", "true")
	os.Setenv(envDebug, "true")
	defer os.Unsetenv("KUSTOMIZE_PLUGIN_CONFIG_STRING")
	out := runKustomizePlugin(t, filepath.Dir(file))
	validateYAML(t, out, 1)
	require.Contains(t, string(out), "\n  key: b\n", "output: %s", string(out))
}

func runKustomizePlugin(t *testing.T, wd string, args ...string) (out []byte) {
	wdOrig, err := os.Getwd()
	require.NoError(t, err)
	os.Setenv("KUSTOMIZE_PLUGIN_CONFIG_ROOT", wdOrig)
	err = os.Chdir(wd)
	require.NoError(t, err)
	defer os.Chdir(wdOrig)
	os.Args = append([]string{"testee"}, args...)
	var buf bytes.Buffer
	err = Execute(&buf)
	require.NoError(t, err)
	return buf.Bytes()
}

func validateYAML(t *testing.T, y []byte, objAmount int) {
	dec := yaml.NewDecoder(bytes.NewReader(y))
	i := -1
	var err error
	for ; err == nil; err = dec.Decode(map[string]interface{}{}) {
		i++
	}
	if err == io.EOF {
		err = nil
	}
	require.NoError(t, err)
	if objAmount >= 0 {
		require.Equal(t, objAmount, i, "amount of resources within output")
	}
	return
}
