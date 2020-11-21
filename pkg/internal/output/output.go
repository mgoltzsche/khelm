package output

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	kustomizeAPIVersion = "kustomize.config.k8s.io/v1beta1"
	kustomizeKind       = "Kustomization"
)

// Options specifies the output options
type Options struct {
	File          string
	Kustomization string
	Replace       bool
	Writer        io.Writer
}

// Output specifies a kubernetes resource sink
type Output interface {
	Write([]*yaml.RNode) error
}

// ResourcePath derives the output path (using / as separator) from a given resource
func ResourcePath(meta yaml.ResourceMeta, basePath string) string {
	name := fmt.Sprintf("%s_%s", strings.ToLower(meta.Kind), meta.Name)
	name = strings.TrimRight(name, "_")
	return path.Join(basePath, fmt.Sprintf("%s.yaml", name))
}

// New creates a new output from the given options
func New(o Options) (Output, error) {
	if o.Kustomization != "" && o.File != "" {
		return nil, errors.New("setting both kustomization output dir and output file is not allowed")
	}
	if o.Kustomization != "" {
		return &kustomizationOutput{dirOutput{o.Kustomization, o.Replace}}, nil
	}
	if o.File != "" {
		return &fileOutput{o.File, o.Replace}, nil
	}
	if o.Replace {
		return nil, errors.New("output replacement cannot be enabled when writing to stdout")
	}
	return &writerOutput{o.Writer}, nil
}

type fileOutput struct {
	file    string
	replace bool
}

func (w *fileOutput) Write(resources []*yaml.RNode) (err error) {
	return writeToFile(resources, w.file, w.replace)
}

type dirOutput struct {
	dir     string
	replace bool
}

func (w *dirOutput) Write(resources []*yaml.RNode) error {
	if w.replace {
		if err := os.RemoveAll(w.dir); err != nil {
			return errors.New(err.Error())
		}
	}
	if err := os.MkdirAll(w.dir, 0755); err != nil {
		return errors.New(err.Error())
	}
	if !w.replace {
		containsFiles, err := containsFiles(w.dir)
		if err != nil {
			return err
		}
		if containsFiles {
			return errors.Errorf("output directory %q already contains files. use --out-replace to delete and recreate the directory", w.dir)
		}
	}
	for _, r := range resources {
		resource := []*yaml.RNode{r}
		var buf bytes.Buffer
		_ = Marshal(resource, &buf)
		raw := strings.ReplaceAll(buf.String(), "\n", "\n  ")
		m, err := r.GetMeta()
		if err != nil {
			return errors.Errorf("invalid output resource metadata: %s\n  provided resource:\n  %s", err.Error(), raw)
		}
		if m.Name == "" {
			return errors.Errorf("output resource has no name:\n  %s", raw)
		}
		outFile := filepath.FromSlash(ResourcePath(m, filepath.ToSlash(w.dir)))
		if err = writeToFile(resource, outFile, false); err != nil {
			return err
		}
	}
	return nil
}

type kustomizationOutput struct {
	dirOutput
}

func (w kustomizationOutput) Write(resources []*yaml.RNode) (err error) {
	if err = w.dirOutput.Write(resources); err != nil {
		return err
	}
	paths := make([]string, len(resources))
	for i, r := range resources {
		m, _ := r.GetMeta()
		paths[i] = ResourcePath(m, "")
	}
	kustomization := map[string]interface{}{}
	kustomization["apiVersion"] = kustomizeAPIVersion
	kustomization["kind"] = kustomizeKind
	kustomization["resources"] = paths
	kustomizationFile := filepath.Join(w.dir, "kustomization.yaml")
	f, err := os.OpenFile(kustomizationFile, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0644)
	defer func() {
		if e := f.Close(); e != nil && err == nil {
			err = errors.Errorf("close file writer: %s", e)
		}
	}()
	b, err := yaml.Marshal(kustomization)
	if err != nil {
		return errors.Wrap(err, "marshal kustomization.yaml")
	}
	_, err = f.Write(b)
	if err != nil {
		return errors.Wrap(err, "write kustomization.yaml")
	}
	return nil
}

type writerOutput struct {
	out io.Writer
}

func (w *writerOutput) Write(resources []*yaml.RNode) error {
	return Marshal(resources, w.out)
}

func writeToFile(resources []*yaml.RNode, outFile string, replace bool) error {
	flags := os.O_CREATE | os.O_WRONLY | os.O_EXCL
	if replace {
		flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}
	f, err := os.OpenFile(outFile, flags, 0644)
	defer func() {
		if e := f.Close(); e != nil && err == nil {
			err = errors.Errorf("close file writer: %s", e)
		}
	}()

	return errors.Wrapf(Marshal(resources, f), "write output file %s", outFile)
}

// Marshal writes the given list of resources as YAML into the given writer
func Marshal(resources []*yaml.RNode, writer io.Writer) error {
	enc := yaml.NewEncoder(writer)
	for _, r := range resources {
		if err := enc.Encode(r.Document()); err != nil {
			return errors.Errorf("marshal: %s", err)
		}
	}
	err := enc.Close()
	return errors.Wrap(err, "close marshaller")
}

func containsFiles(dir string) (bool, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		files = append(files, path)
		return nil
	})
	return len(files) > 1, errors.Wrap(err, "preflight output dir check")
}
