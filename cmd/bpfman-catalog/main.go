package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/openshift/bpfman-catalog/pkg/analysis"
	"github.com/openshift/bpfman-catalog/pkg/bundle"
	"github.com/openshift/bpfman-catalog/pkg/manifests"
	"github.com/openshift/bpfman-catalog/pkg/writer"
)

// Default output directories
const (
	DefaultArtefactsDir = "auto-generated/artefacts"
	DefaultManifestsDir = "auto-generated/manifests"
)

// GlobalContext contains global dependencies injected into commands.
type GlobalContext struct {
	Context context.Context
	Logger  *slog.Logger
}

// CLI defines the command-line interface structure.
type CLI struct {
	PrepareCatalogBuildFromBundle     PrepareCatalogBuildFromBundleCmd     `cmd:"prepare-catalog-build-from-bundle" help:"Prepare catalog build artefacts from a bundle image"`
	PrepareCatalogBuildFromYAML       PrepareCatalogBuildFromYAMLCmd       `cmd:"prepare-catalog-build-from-yaml" help:"Prepare catalog build artefacts from an existing catalog.yaml file"`
	PrepareCatalogDeploymentFromImage PrepareCatalogDeploymentFromImageCmd `cmd:"prepare-catalog-deployment-from-image" help:"Prepare deployment manifests from existing catalog image"`
	BundleInfo                        BundleInfoCmd                        `cmd:"bundle-info" help:"Show bundle contents and dependencies"`
	ListBundles                       ListBundlesCmd                       `cmd:"list-bundles" help:"List available bundle images"`

	// Global flags
	LogLevel  string `env:"LOG_LEVEL" default:"info" help:"Log level (debug, info, warn, error)"`
	LogFormat string `env:"LOG_FORMAT" default:"text" help:"Log format (text, json)"`
}

// PrepareCatalogBuildFromBundleCmd prepares catalog build artefacts from a bundle image.
type PrepareCatalogBuildFromBundleCmd struct {
	BundleImage string `arg:"" required:"" help:"Bundle image reference"`
	OutputDir   string `default:"${default_artefacts_dir}" help:"Output directory for generated artefacts"`
	OpmBin      string `type:"path" help:"Path to opm binary for external rendering (uses library by default)"`
}

// PrepareCatalogBuildFromYAMLCmd prepares catalog build artefacts from existing catalog.yaml.
type PrepareCatalogBuildFromYAMLCmd struct {
	CatalogYAML string `arg:"" type:"path" required:"" help:"Path to existing catalog.yaml file"`
	OutputDir   string `default:"${default_artefacts_dir}" help:"Output directory for generated artefacts"`
}

// PrepareCatalogDeploymentFromImageCmd prepares deployment manifests from catalog image.
type PrepareCatalogDeploymentFromImageCmd struct {
	CatalogImage string `arg:"" required:"" help:"Catalog image reference"`
	OutputDir    string `default:"${default_manifests_dir}" help:"Output directory for generated manifests"`
}

// BundleInfoCmd shows bundle contents and dependencies.
type BundleInfoCmd struct {
	BundleImages []string `arg:"" required:"" help:"Bundle image references"`
	Format       string   `default:"text" enum:"text,json" help:"Output format (text, json)"`
}

// ListBundlesCmd lists available bundle images.
type ListBundlesCmd struct {
	Repository string `help:"Bundle repository (default: quay.io/redhat-user-workloads/ocp-bpfman-tenant/bpfman-operator-bundle-ystream)"`
	List       int    `default:"5" help:"Number of latest bundles to list"`
	Format     string `default:"text" enum:"text,json" help:"Output format (text, json)"`
}

func (r *PrepareCatalogBuildFromBundleCmd) Run(globals *GlobalContext) error {
	if filepath.Clean(r.OutputDir) == "." {
		return fmt.Errorf("output directory cannot be the current working directory, please specify a named subdirectory like '%s'", DefaultArtefactsDir)
	}

	if err := os.RemoveAll(r.OutputDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cleaning output directory: %w", err)
	}

	var gen *bundle.Generator
	if r.OpmBin != "" {
		gen = bundle.NewGeneratorWithOmp(r.BundleImage, "preview", r.OpmBin)
	} else {
		gen = bundle.NewGenerator(r.BundleImage, "preview")
	}

	artefacts, err := gen.Generate(globals.Context)
	if err != nil {
		return fmt.Errorf("generating bundle artefacts: %w", err)
	}

	w := writer.New(r.OutputDir)

	if err := w.WriteSingle("fbc-template.yaml", []byte(artefacts.FBCTemplate)); err != nil {
		return fmt.Errorf("writing FBC template: %w", err)
	}

	if artefacts.CatalogYAML != "" {
		if err := w.WriteSingle("catalog.yaml", []byte(artefacts.CatalogYAML)); err != nil {
			return fmt.Errorf("writing catalog: %w", err)
		}
	}

	if err := w.WriteSingle("Dockerfile", []byte(artefacts.Dockerfile)); err != nil {
		return fmt.Errorf("writing Dockerfile: %w", err)
	}
	if err := w.WriteSingle("Makefile", []byte(artefacts.Makefile)); err != nil {
		return fmt.Errorf("writing Makefile: %w", err)
	}

	catalogRendered := artefacts.CatalogYAML != ""
	imageUUID, randomTTL := bundle.GenerateImageUUIDAndTTL()
	workflow := bundle.GenerateWorkflow(0, catalogRendered, r.OutputDir, imageUUID, randomTTL)
	if err := w.WriteSingle("WORKFLOW.txt", []byte(workflow)); err != nil {
		return fmt.Errorf("writing WORKFLOW.txt: %w", err)
	}

	fmt.Print(workflow)
	fmt.Printf("\nThis information is saved in %s/WORKFLOW.txt\n", r.OutputDir)
	return nil
}

func (r *PrepareCatalogBuildFromYAMLCmd) Run(globals *GlobalContext) error {
	if filepath.Clean(r.OutputDir) == "." {
		return fmt.Errorf("output directory cannot be the current working directory, please specify a named subdirectory like '%s'", DefaultArtefactsDir)
	}

	if err := os.RemoveAll(r.OutputDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cleaning output directory: %w", err)
	}

	catalogContent, err := os.ReadFile(r.CatalogYAML)
	if err != nil {
		return fmt.Errorf("reading catalog.yaml: %w", err)
	}

	w := writer.New(r.OutputDir)

	if err := w.WriteSingle("catalog.yaml", catalogContent); err != nil {
		return fmt.Errorf("writing catalog.yaml: %w", err)
	}

	dockerfile := bundle.GenerateCatalogDockerfile()
	if err := w.WriteSingle("Dockerfile", []byte(dockerfile)); err != nil {
		return fmt.Errorf("writing Dockerfile: %w", err)
	}

	execPath, err := os.Executable()
	if err != nil {
		execPath = "bpfman-catalog"
	}

	imageUUID, randomTTL := bundle.GenerateImageUUIDAndTTL()

	makefile := bundle.GenerateMakefile("from-yaml", execPath, imageUUID, randomTTL)
	if err := w.WriteSingle("Makefile", []byte(makefile)); err != nil {
		return fmt.Errorf("writing Makefile: %w", err)
	}

	workflow := bundle.GenerateWorkflow(0, true, r.OutputDir, imageUUID, randomTTL)
	if err := w.WriteSingle("WORKFLOW.txt", []byte(workflow)); err != nil {
		return fmt.Errorf("writing WORKFLOW.txt: %w", err)
	}

	fmt.Print(workflow)
	fmt.Printf("\nThis information is saved in %s/WORKFLOW.txt\n", r.OutputDir)

	return nil
}

func (r *PrepareCatalogDeploymentFromImageCmd) Run(globals *GlobalContext) error {
	if filepath.Clean(r.OutputDir) == "." {
		return fmt.Errorf("output directory cannot be the current working directory, please specify a named subdirectory like '%s'", DefaultManifestsDir)
	}

	if err := os.RemoveAll(r.OutputDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cleaning output directory: %w", err)
	}

	config := manifests.GeneratorConfig{
		Namespace:     "bpfman",
		UseDigestName: true,
		ImageRef:      r.CatalogImage,
	}

	generator := manifests.NewGenerator(config)

	manifestSet, err := generator.GenerateFromCatalog(globals.Context)
	if err != nil {
		return fmt.Errorf("generating manifests: %w", err)
	}

	writer := writer.New(r.OutputDir)
	if err := writer.WriteAll(manifestSet); err != nil {
		return fmt.Errorf("writing manifests: %w", err)
	}

	fmt.Printf("Manifests generated in %s\n", r.OutputDir)
	return nil
}

func (r *BundleInfoCmd) Run(globals *GlobalContext) error {
	for _, bundleImage := range r.BundleImages {
		result, err := analysis.AnalyseBundle(globals.Context, bundleImage)
		if err != nil {
			return fmt.Errorf("failed to analyse bundle %s: %w", bundleImage, err)
		}

		output, err := analysis.FormatResult(result, r.Format)
		if err != nil {
			return fmt.Errorf("failed to format output for %s: %w", bundleImage, err)
		}

		fmt.Print(output)
	}

	return nil
}

func (r *ListBundlesCmd) Run(globals *GlobalContext) error {
	var bundleRef bundle.BundleRef
	var err error

	if r.Repository != "" {
		bundleRef, err = bundle.ParseBundleRef(r.Repository)
		if err != nil {
			return fmt.Errorf("parsing repository: %w", err)
		}
	} else {
		bundleRef = bundle.NewDefaultBundleRef()
	}

	bundles, err := bundle.ListLatestBundles(globals.Context, bundleRef, r.List)
	if err != nil {
		return fmt.Errorf("listing bundles: %w", err)
	}

	if r.Format == "json" {
		output, err := formatBundlesJSON(bundles)
		if err != nil {
			return fmt.Errorf("formatting JSON output: %w", err)
		}
		fmt.Println(output)
	} else {
		formatBundlesText(bundles)
	}

	return nil
}

func formatBundlesText(bundles []*bundle.BundleMetadata) {
	for _, b := range bundles {
		imageBase := b.Image[:strings.LastIndex(b.Image, ":")]
		gitCommitShort := b.Tag
		if len(gitCommitShort) > 8 {
			gitCommitShort = gitCommitShort[:8]
		}
		fmt.Printf("%s@%s %s g%s\n", imageBase, b.Digest, b.BuildDate, gitCommitShort)
	}
}

func formatBundlesJSON(bundles []*bundle.BundleMetadata) (string, error) {
	type output struct {
		Count   int                      `json:"count"`
		Bundles []*bundle.BundleMetadata `json:"bundles"`
	}

	out := output{
		Count:   len(bundles),
		Bundles: bundles,
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func printWorkflowGuide() {
	fmt.Printf(`
Workflows:

1. Build catalog from a bundle image
   Generates complete build artefacts from a bundle

   # Generates: Dockerfile, catalog.yaml, Makefile
   $ bpfman-catalog prepare-catalog-build-from-bundle \
       quay.io/redhat-user-workloads/ocp-bpfman-tenant/bpfman-operator-bundle-ystream:latest

   # builds image, pushes to registry, deploys to cluster
   $ make -C %s all

2. Build catalog from catalog.yaml
   Wraps an existing or modified catalog.yaml with build artefacts:

   # Generates: Dockerfile, Makefile
   $ bpfman-catalog prepare-catalog-build-from-yaml auto-generated/catalog/y-stream.yaml

   # builds image, pushes to registry, deploys to cluster
   $ make -C %s all

3. Deploy existing catalog image
   Generates Kubernetes manifests to deploy a catalog to a cluster

   # Produces: CatalogSource, Namespace, IDMS
   $ bpfman-catalog prepare-catalog-deployment-from-image \
       quay.io/redhat-user-workloads/ocp-bpfman-tenant/catalog-ystream:latest

   # Deploy catalog to cluster
   $ kubectl apply -f %s/
`, DefaultArtefactsDir, DefaultArtefactsDir, DefaultManifestsDir)
}

func main() {
	var cli CLI

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Check if help was requested to print workflow guide
	showWorkflowGuide := false
	for _, arg := range os.Args[1:] {
		if arg == "--help" || arg == "-h" {
			showWorkflowGuide = true
			break
		}
	}

	kongCtx := kong.Parse(&cli,
		kong.Name("bpfman-catalog"),
		kong.Description("Deploy and manage bpfman operator catalogs on OpenShift"),
		kong.UsageOnError(),
		kong.Vars{
			"default_artefacts_dir": DefaultArtefactsDir,
			"default_manifests_dir": DefaultManifestsDir,
		},
		kong.Exit(func(code int) {
			// Print workflow guide before exiting on help
			if showWorkflowGuide && len(os.Args) == 2 {
				printWorkflowGuide()
			}
			os.Exit(code)
		}),
	)

	// Print workflow guide after Kong help for non-exit cases
	if showWorkflowGuide && len(os.Args) == 2 {
		printWorkflowGuide()
	}

	logger := setupLogger(cli.LogLevel, cli.LogFormat)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	globals := &GlobalContext{
		Context: ctx,
		Logger:  logger,
	}

	errChan := make(chan error, 1)
	go func() {
		errChan <- kongCtx.Run(globals)
	}()

	select {
	case sig := <-sigChan:
		logger.Debug("received signal", slog.String("signal", sig.String()))
		cancel()
		if err := <-errChan; err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			logger.Debug("command failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	case err := <-errChan:
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			logger.Debug("command failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}
}

func setupLogger(level, format string) *slog.Logger {
	logLevel := parseLogLevel(level)

	opts := &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: logLevel == slog.LevelDebug,
	}

	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	return slog.New(handler)
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug", "trace":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
