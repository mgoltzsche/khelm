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
	"gopkg.in/yaml.v3"
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
		name              string
		file              string
		expectedNamespace string
		expectedContained string
	}{
		{"jenkins", "../../example/jenkins/jenkins-chart.yaml", "jenkins", expectedJenkinsContained},
		{"values-external", "chartwithextvalues.yaml", "jenkins", expectedJenkinsContained},
		{"rook-ceph", "../../example/rook-ceph/operator/rook-ceph-chart.yaml", "rook-ceph-system", "rook-ceph-v0.9.3"},
		{"cert-manager", "../../example/cert-manager/cert-manager-chart.yaml", "cert-manager", "chart: cainjector-v0.9.1"},
		{"apiversions-condition", "../../example/apiversions-condition/chartref.yaml", "apiversions-condition-env", "  config: fancy-config"},
		{"local-chart-with-remote-dependency", "../../example/localref/chartref.yaml", "myns", "elasticsearch"},
		{"local-chart-with-local-dependency", "../../example/localrefref/chartref.yaml", "myotherns", "elasticsearch"},
		{"values-inheritance", "../../example/values-inheritance/chartref.yaml", "values-inheritance-env", "<inherited:inherited value> <fileoverwrite:overwritten by file> <valueoverwrite:overwritten by generator config>"},
		{"unsupported-field", "../../example/unsupported-field/chartref.yaml", "rook-ceph-system", "rook-ceph"},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, cached := range []string{"", "cached "} {
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
					if o["metadata"].(map[string]interface{})["namespace"] == c.expectedNamespace {
						hasExpectedNamespace = true
						break
					}
				}
				require.True(t, hasExpectedNamespace, "%s%s: should have namespace %q", cached, c.file, c.expectedNamespace)
			}
		})
	}
}

func TestRenderRejectFileOutsideProjectDir(t *testing.T) {
	file := filepath.Join(currDir, "chartwithextvalues.yaml")
	err := render(t, file, currDir, &bytes.Buffer{})
	require.Error(t, err, "render %s within %s", file, currDir)
}

func TestRenderError(t *testing.T) {
	for _, file := range []string{
		"../../example/invalid-requirements-lock/chartref.yaml",
	} {
		file = filepath.Join(currDir, file)
		rootDir := filepath.Join(currDir, "..", "..")
		err := render(t, file, rootDir, &bytes.Buffer{})
		require.Error(t, err, "render %s", file)
	}
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
