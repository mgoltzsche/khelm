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
	log.SetFlags(0)
	chdir(filepath.Join(currDir, "../../example/jenkins"))
	defer chdir(currDir)
	var rendered bytes.Buffer
	f, err := os.Open("jenkins-chart.yaml")
	require.NoError(t, err)
	defer f.Close()
	cfg, err := ReadGeneratorConfig(f)
	require.NoError(t, err)
	err = Render(cfg, &rendered)
	require.NoError(t, err, "render")
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

func chdir(dir string) {
	if err := os.Chdir(dir); err != nil {
		panic(err)
	}
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
