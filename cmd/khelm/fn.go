package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/mgoltzsche/khelm/v2/internal/matcher"
	"github.com/mgoltzsche/khelm/v2/internal/output"
	"github.com/mgoltzsche/khelm/v2/pkg/config"
	"github.com/mgoltzsche/khelm/v2/pkg/helm"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"sigs.k8s.io/kustomize/kyaml/fn/framework"
	"sigs.k8s.io/kustomize/kyaml/fn/framework/command"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	apiVersionConfigKubernetesIO = "config.kubernetes.io"
	annotationIndex              = apiVersionConfigKubernetesIO + "/index"
	annotationPath               = apiVersionConfigKubernetesIO + "/path"
	defaultOutputPath            = "generated-manifest.yaml"
	envOutputPath                = "KHELM_OUTPUT_PATH"
	envChart                     = "KHELM_BUILTIN_CHART"
	envRepository                = "KHELM_BUILTIN_REPOSITORY"
	envKind                      = "KHELM_KIND"
	envApiVersion                = "KHELM_APIVERSION"
)

func krmFnCommand(h *helm.Helm) *cobra.Command {
	processor := framework.ResourceListProcessorFunc(func(resourceList *framework.ResourceList) (err error) {
		if resourceList.FunctionConfig.IsNilOrEmpty() {
			return fmt.Errorf("no function config specified")
		}
		fnCfg, err := loadKRMFunctionConfig(resourceList)
		if err != nil {
			return errors.Wrap(err, "load khelm function config")
		}
		outputPath := fnCfg.OutputPath
		if outputPath == "" {
			outputPath = defaultOutputPath
		}
		outputPaths := make([]string, len(fnCfg.OutputPathMapping)+1)
		outputPaths[0] = outputPath
		for i, m := range fnCfg.OutputPathMapping {
			outputPaths[i+1] = m.OutputPath
			if m.OutputPath == "" {
				return errors.Errorf("no outputPath specified for outputPathMapping[%d]", i)
			}
			if len(m.Selectors) == 0 {
				return errors.Errorf("no selectors specified for outputPathMapping[%d] -> %q", i, m.OutputPath)
			}
		}

		// Template the helm chart
		h.Settings.Debug = h.Settings.Debug || fnCfg.Debug
		rendered, err := render(h, &fnCfg.ChartConfig)
		if err != nil {
			return err
		}

		// Apply output path mappings and annotate resources
		kustomizationDirs, err := mapOutputPaths(rendered, fnCfg.OutputPathMapping, outputPath, h.Settings.Debug)
		if err != nil {
			return err
		}

		// Generate kustomizations
		dirs := make([]string, 0, len(kustomizationDirs))
		for dir := range kustomizationDirs {
			dirs = append(dirs, dir)
		}
		sort.Strings(dirs)
		kustomizationResources := make([]*yaml.RNode, len(dirs))
		for i, dir := range dirs {
			resources := kustomizationDirs[dir]
			kustomizationPath := path.Join(dir, "kustomization.yaml")
			kustomization, err := generateKustomization(resources)
			if err != nil {
				return errors.Wrapf(err, "generate %s", kustomizationPath)
			}
			err = setKptAnnotations(kustomization, kustomizationPath, 0, h.Settings.Debug)
			if err != nil {
				return errors.Wrap(err, "set kpt annotations on kustomization")
			}
			kustomizationResources[i] = kustomization
		}
		rendered = append(kustomizationResources, rendered...)

		// Apply output
		resourceList.Items = filterByOutputPath(resourceList.Items, outputPaths)
		resourceList.Items = append(resourceList.Items, rendered...)
		return nil
	})
	return command.Build(processor, command.StandaloneEnabled, false)
}

func loadKRMFunctionConfig(rl *framework.ResourceList) (*config.KRMFuncConfig, error) {
	cfg := &config.KRMFuncConfig{}
	fnKind := rl.FunctionConfig.GetKind()
	fnApiVersion := rl.FunctionConfig.GetApiVersion()
	outputPath := os.Getenv(envOutputPath)
	builtInChart := os.Getenv(envChart)
	builtInRepo := os.Getenv(envRepository)
	expectKind := os.Getenv(envKind)
	expectAPIVersion := os.Getenv(envApiVersion)
	if expectKind == "" || expectAPIVersion == "" {
		if builtInChart != "" {
			return nil, fmt.Errorf("must also specify KHELM_KIND, KHELM_APIVERSION and KHELM_OUTPUT_PATH when specifying KHELM_BUILTIN_CHART")
		}
		expectKind = config.GeneratorKind
		expectAPIVersion = config.GeneratorAPIVersion
	}
	fnCfg := &config.KRMFuncConfigFile{KRMFuncConfig: *cfg}
	cfg = &fnCfg.KRMFuncConfig
	isOldGeneratorKind := fnKind == config.GeneratorKind && fnApiVersion == config.GeneratorAPIVersion
	isConfigMap := fnKind == "ConfigMap" && fnApiVersion == "v1"
	isExpectedKind := fnKind == expectKind && fnApiVersion == expectAPIVersion
	if !isExpectedKind && !isConfigMap {
		return nil, fmt.Errorf("unsupported kind %q and apiversion %q provided, expecting kind %q and apiversion %q", fnKind, fnApiVersion, expectKind, expectAPIVersion)
	}
	err := framework.LoadFunctionConfig(rl.FunctionConfig, fnCfg)
	if err != nil {
		return nil, err
	}
	if fnCfg.Data != nil {
		if !isOldGeneratorKind && !isConfigMap {
			return nil, fmt.Errorf("unsupported field: data")
		}
		if isOldGeneratorKind {
			log.Printf("WARNING: data field is deprecated within %s. please specify fields at root level", config.GeneratorKind)
		}
		fnCfg.KRMFuncConfig = *fnCfg.Data
		fnCfg.Data = nil
	}
	if cfg.Name == "" {
		cfg.Name = fnCfg.Metadata.Name
	}
	if cfg.Namespace == "" {
		cfg.Namespace = fnCfg.Metadata.Namespace
	}
	if cfg.OutputPath == "" {
		cfg.OutputPath = outputPath
	}
	cfg.ApplyDefaults()
	if builtInChart != "" {
		if cfg.Chart != "" || cfg.Repository != "" {
			return nil, fmt.Errorf("cannot specify chart or repository when invoking the khelm function declaratively (KHELM_BUILTIN_CHART is set)")
		}
		cfg.Chart = builtInChart
		cfg.Repository = builtInRepo
	}
	return cfg, nil
}

func generateKustomization(resources []*yaml.RNode) (*yaml.RNode, error) {
	resourcePaths := resourceNames(resources, "")
	m := map[string]interface{}{
		"apiVersion": "kustomize.config.k8s.io/v1beta1",
		"kind":       "Kustomization",
		"resources":  resourcePaths,
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return yaml.ConvertJSONToYamlNode(string(b))
}

func filterByOutputPath(resources []*yaml.RNode, outputPaths []string) []*yaml.RNode {
	r := make([]*yaml.RNode, 0, len(resources))
	for _, o := range resources {
		meta, err := o.GetMeta()
		if err != nil || meta.Annotations == nil || !isGeneratedOutputPath(meta.Annotations[annotationPath], outputPaths) {
			r = append(r, o)
		}
	}
	return r
}

func isGeneratedOutputPath(path string, outputPaths []string) bool {
	for _, p := range outputPaths {
		if path == p || output.IsDirectory(p) && strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func mapOutputPaths(resources []*yaml.RNode, outputMappings []config.KRMFuncOutputMapping, defaultOutputPath string, debug bool) (map[string][]*yaml.RNode, error) {
	matchers := make([]matcher.ResourceMatchers, len(outputMappings))
	for i, m := range outputMappings {
		matchers[i] = matcher.FromResourceSelectors(m.Selectors)
	}
	kustomizationDirs := map[string][]*yaml.RNode{}
	for i, o := range resources {
		meta, err := o.GetMeta()
		if err != nil {
			continue
		}

		outPath := defaultOutputPath
		for i, m := range matchers {
			if m.Match(&meta) {
				outPath = outputMappings[i].OutputPath
				break
			}
		}

		// Set kpt order and path annotations
		if output.IsDirectory(outPath) {
			kustomizationDirs[outPath] = append(kustomizationDirs[outPath], o)
			outPath = output.ResourcePath(meta, outPath)
		}
		err = setKptAnnotations(o, outPath, i, debug)
		if err != nil {
			return nil, errors.Wrapf(err, "set annotations on %s/%s", meta.Kind, meta.Name)
		}
	}

	for _, m := range matchers {
		err := m.RequireAllMatched()
		if err != nil {
			return nil, errors.Wrap(err, "outputPathMapping")
		}
	}

	return kustomizationDirs, nil
}

func setKptAnnotations(o *yaml.RNode, path string, index int, debug bool) error {
	if debug {
		m, err := o.GetMeta()
		if err != nil {
			return err
		}
		log.Printf("Mapping %s %s to path %s", m.Kind, m.Name, path)
	}
	// Remove annotations field if empty.
	// This is required because LookupCreate() doesn't create the MappingNode if it exists but is empty (#13).
	err := o.PipeE(yaml.LookupCreate(yaml.MappingNode, yaml.MetadataField), yaml.FieldClearer{Name: yaml.AnnotationsField, IfEmpty: true})
	if err != nil {
		return err
	}
	// Add path annotations
	lookupAnnotations := yaml.LookupCreate(yaml.MappingNode, yaml.MetadataField, yaml.AnnotationsField)
	err = o.PipeE(lookupAnnotations, yaml.FieldSetter{Name: annotationIndex, StringValue: strconv.Itoa(index)})
	if err != nil {
		return err
	}
	err = o.PipeE(lookupAnnotations, yaml.FieldSetter{Name: annotationPath, StringValue: path})
	if err != nil {
		return err
	}
	return nil
}

func resourceNames(resources []*yaml.RNode, outputBasePath string) []string {
	outPaths := make([]string, 0, len(resources))
	for _, o := range resources {
		if meta, err := o.GetMeta(); err == nil {
			outPaths = append(outPaths, output.ResourcePath(meta, outputBasePath))
		}
	}
	return outPaths
}
