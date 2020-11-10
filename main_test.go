package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func Test_traceableRootCause(t *testing.T) {
	simpleErr := fmt.Errorf("simple error")
	traceableErr := errors.New("traceable error")
	wrapped := errors.Wrap(simpleErr, "wrapped")
	msgErr := errors.WithMessage(simpleErr, "msg")
	wrappedMsg := errors.Wrap(msgErr, "wrapped")
	tests := []struct {
		name     string
		input    error
		expected error
	}{
		{"no wrapping or causing", simpleErr, simpleErr},
		{"wrapping", fmt.Errorf("wrapped: %w", traceableErr), traceableErr},
		{"causing", errors.Wrap(traceableErr, "wrapped"), traceableErr},
		{"deeply nested", errors.Wrap(fmt.Errorf("errorf: %w", errors.Wrap(traceableErr, "wrappedinner")), "wrappedouter"), traceableErr},
		{"root cause without stack trace", errors.Wrap(wrapped, "wrappedouter"), wrapped},
		{"root cause without stack trace but formatter", errors.Wrap(wrappedMsg, "wrappedouter"), wrappedMsg},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := traceableRootCause(tt.input)
			require.Equal(t, tt.expected, err)
		})
	}
}

func TestMain(t *testing.T) {
	file := filepath.Join("example", "exclude", "chartref.yaml")
	kustomizeGenCfg, err := ioutil.ReadFile(file)
	require.NoError(t, err)
	os.Setenv("KUSTOMIZE_PLUGIN_CONFIG_STRING", string(kustomizeGenCfg))
	os.Setenv("HELMR_ALLOW_UNKNOWN_REPOSITORIES", "true")
	os.Setenv(envDebug, "true")
	defer os.Unsetenv("KUSTOMIZE_PLUGIN_CONFIG_STRING")
	out := runMain(t, filepath.Dir(file))
	validateYAML(t, out, 2)
	require.Contains(t, string(out), "\n  key: b\n", "output: %s", string(out))
}

func runMain(t *testing.T, wd string, args ...string) (out []byte) {
	dir, err := ioutil.TempDir("", "helmkustomizeplugintestdir-")
	require.NoError(t, err)
	defer os.RemoveAll(dir)
	wdOrig, err := os.Getwd()
	require.NoError(t, err)
	os.Setenv("KUSTOMIZE_PLUGIN_CONFIG_ROOT", wdOrig)
	err = os.Chdir(wd)
	require.NoError(t, err)
	defer os.Chdir(wdOrig)
	os.Args = append([]string{"testee"}, args...)
	stdout := os.Stdout
	outFile := filepath.Join(dir, "rendered.yaml")
	f, err := os.OpenFile(outFile, os.O_CREATE|os.O_WRONLY, 0644)
	require.NoError(t, err)
	defer f.Close()
	os.Stdout = f
	defer func() {
		os.Stdout = stdout
	}()
	defer func() {
		out, err = ioutil.ReadFile(outFile)
		require.NoError(t, err, "read mocked stdout file")
	}()
	main()
	return
}

func validateYAML(t *testing.T, y []byte, objAmount int) (err error) {
	dec := yaml.NewDecoder(bytes.NewReader(y))
	i := 0
	for ; err == nil; err = dec.Decode(map[string]interface{}{}) {
		i++
	}
	if err != io.EOF {
		err = nil
	}
	require.Equal(t, objAmount, i, "amount of resources within output")
	return
}
