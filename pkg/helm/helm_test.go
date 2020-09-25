package helm

import (
	"bytes"
	"context"
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
	expectedJenkinsContained := "- host: jenkins.example.org\n"
	for _, c := range []struct {
		file                string
		expectedNamespace   string
		expectedContained   string
		featureFlagGoGetter bool
	}{
		{"../../example/jenkins/jenkins-chart.yaml", "jenkins", expectedJenkinsContained, false},
		{"chartwithextvalues.yaml", "jenkins", expectedJenkinsContained, false},
		{"../../example/rook-ceph/operator/rook-ceph-chart.yaml", "rook-ceph-system", "rook-ceph-v0.9.3", false},
		{"../../example/rook-ceph/operator/rook-ceph-chart.yaml", "rook-ceph-system", "rook-ceph-v0.9.3", false},
		{"../../example/conditionalcr/chartref.yaml", "conditionalcrenv", "  config: fancy-config", false},
		{"../../example/localref/chartref.yaml", "myns", "elasticsearch", true},
		{"../../example/gitref/chartref.yaml", "linkerd", "linkerd", true},
	} {
		for _, cached := range []string{"", "cached "} {
			featureFlagGoGetter = c.featureFlagGoGetter
			var rendered bytes.Buffer
			absFile := filepath.Join(currDir, c.file)
			rootDir := filepath.Join(currDir, "..", "..")
			err := render(t, absFile, rootDir, &rendered)
			require.NoError(t, err, "render %s%s", cached, absFile)
			b := rendered.Bytes()
			l, err := readYaml(b)
			require.NoError(t, err, "rendered %syaml:\n%s", cached, b)
			require.True(t, len(l) > 0, "%s: rendered result of %s is empty", cached, c.file)
			require.Contains(t, rendered.String(), c.expectedContained, "%syaml", cached)
			hasExpectedNamespace := false
			for _, o := range l {
				if o["metadata"].(map[interface{}]interface{})["namespace"] == c.expectedNamespace {
					hasExpectedNamespace = true
					break
				}
			}
			require.True(t, hasExpectedNamespace, "%s%s: should have namespace %q", cached, c.file, c.expectedNamespace)
		}
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
	err = Render(context.Background(), cfg, writer)
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
