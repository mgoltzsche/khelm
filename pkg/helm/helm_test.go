package helm

import (
	"bytes"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

var currDir = func() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return wd
}()

func TestRender(t *testing.T) {
	for _, file := range []string{
		"../../example/jenkins/jenkins-chart.yaml",
		"chartwithextvalues.yaml",
	} {
		var rendered bytes.Buffer
		absFile := filepath.Join(currDir, file)
		rootDir := filepath.Join(currDir, "..", "..")
		err := render(t, absFile, rootDir, &rendered)
		require.NoError(t, err, "render %s", file)
		b := rendered.Bytes()
		l, err := readYaml(b)
		require.NoError(t, err, "rendered yaml:\n%s", b)
		require.True(t, len(l) > 0, "rendered yaml is empty")
		require.Contains(t, rendered.String(), "- host: jenkins.example.org\n")
		hasJenkinsNamespace := false
		for _, o := range l {
			if o["metadata"].(map[interface{}]interface{})["namespace"] == "jenkins" {
				hasJenkinsNamespace = true
				break
			}
		}
		require.True(t, hasJenkinsNamespace, "should have 'jenkins' namespace")
	}
}

func TestRenderReject(t *testing.T) {
	file := filepath.Join(currDir, "chartwithextvalues.yaml")
	err := render(t, file, currDir, &bytes.Buffer{})
	require.Error(t, err, "render %s within %s", file, currDir)
}

func render(t *testing.T, file, rootDir string, writer io.Writer) (err error) {
	log.SetFlags(0)
	f, err := os.Open(file)
	require.NoError(t, err)
	defer f.Close()
	cfg, err := ReadGeneratorConfig(f)
	require.NoError(t, err)
	cfg.RootDir = rootDir
	cfg.BaseDir = filepath.Dir(file)
	err = Render(cfg, writer)
	return
}

func readYaml(y []byte) (l []map[string]interface{}, err error) {
	dec := yaml.NewDecoder(bytes.NewReader(y))
	o := map[string]interface{}{}
	for ; err == nil; err = dec.Decode(o) {
		if len(o) > 0 {
			l = append(l, o)
			o = map[string]interface{}{}
		}
	}
	if err == io.EOF {
		err = nil
	}
	return
}
