package main

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
	file := filepath.Join("..", "..", "example", "exclude", "generator.yaml")
	kustomizeGenCfg, err := ioutil.ReadFile(file)
	require.NoError(t, err)
	os.Setenv(envKustomizePluginConfig, string(kustomizeGenCfg))
	os.Setenv(envTrustAnyRepo, "true")
	os.Setenv(envDebug, "true")
	defer os.Unsetenv(envKustomizePluginConfig)
	defer os.Unsetenv(envTrustAnyRepo)
	defer os.Unsetenv(envDebug)
	out := runKustomizePlugin(t, filepath.Dir(file))
	validateYAML(t, out, 1)
	require.Contains(t, string(out), "\n  key: b\n", "output: %s", string(out))
}

func runKustomizePlugin(t *testing.T, wd string, args ...string) (out []byte) {
	wdOrig, err := os.Getwd()
	require.NoError(t, err)
	os.Setenv(envKustomizePluginConfigRoot, wdOrig)
	defer os.Unsetenv(envKustomizePluginConfigRoot)
	err = os.Chdir(wd)
	require.NoError(t, err)
	defer os.Chdir(wdOrig)
	os.Args = append([]string{"testee"}, args...)
	var buf bytes.Buffer
	err = Execute(nil, &buf)
	require.NoError(t, err)
	return buf.Bytes()
}

func validateYAML(t *testing.T, y []byte, objAmount int) (first map[string]interface{}) {
	dec := yaml.NewDecoder(bytes.NewReader(y))
	i := -1
	var err error
	for err == nil {
		o := map[string]interface{}{}
		err = dec.Decode(o)
		if first == nil {
			first = o
		}
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
