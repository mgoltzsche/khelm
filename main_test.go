package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMain(t *testing.T) {
	dir, err := ioutil.TempDir("", "helmkustomizeplugintestdir-")
	require.NoError(t, err)
	defer os.RemoveAll(dir)
	file := filepath.Join("example", "jenkins", "jenkins-chart.yaml")
	file, err = filepath.Abs(file)
	require.NoError(t, err)
	outFile := filepath.Join(dir, "rendered.yaml")
	runMain(outFile, filepath.Dir(file), file)
	b, err := ioutil.ReadFile(outFile)
	require.NoError(t, err)
	require.Contains(t, string(b), "- host: jenkins.example.org\n")
}

func runMain(outFile string, wd string, args ...string) {
	wdOrig, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	os.Setenv("KUSTOMIZE_PLUGIN_CONFIG_ROOT", wdOrig)
	err = os.Chdir(wd)
	if err != nil {
		panic(err)
	}
	defer os.Chdir(wdOrig)
	os.Args = append([]string{"testee"}, args...)
	stdout := os.Stdout
	f, err := os.OpenFile(outFile, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	os.Stdout = f
	defer func() {
		os.Stdout = stdout
	}()
	main()
}
