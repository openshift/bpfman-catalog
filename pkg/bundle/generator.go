package bundle

import (
	"context"
	"fmt"
	"os"

	"sigs.k8s.io/yaml"
)

// Artefacts contains generated files for building a catalog from a bundle.
type Artefacts struct {
	FBCTemplate string // FBC template YAML
	CatalogYAML string // Rendered catalog (if opm is available)
	Dockerfile  string // Dockerfile for building catalog image
	Makefile    string // Makefile for building and deploying catalog
}

// Generator handles bundle to catalog conversion.
type Generator struct {
	bundleImage string
	channel     string
	ompBinPath  string // Optional path to external opm binary
}

// NewGenerator creates a new bundle generator.
func NewGenerator(bundleImage, channel string) *Generator {
	if channel == "" {
		channel = "preview"
	}
	return &Generator{
		bundleImage: bundleImage,
		channel:     channel,
	}
}

// NewGeneratorWithOmp NewGeneratorWithOpm creates a new bundle generator with external
// opm binary.
func NewGeneratorWithOmp(bundleImage, channel, ompBinPath string) *Generator {
	if channel == "" {
		channel = "preview"
	}
	return &Generator{
		bundleImage: bundleImage,
		channel:     channel,
		ompBinPath:  ompBinPath,
	}
}

// Generate creates all artefacts needed to build a catalog from a
// bundle.
func (g *Generator) Generate(ctx context.Context) (*Artefacts, error) {
	fbcTemplate, err := GenerateFBCTemplate(g.bundleImage, g.channel)
	if err != nil {
		return nil, fmt.Errorf("generating FBC template: %w", err)
	}

	fbcYAML, err := yaml.Marshal(fbcTemplate)
	if err != nil {
		return nil, fmt.Errorf("marshaling FBC template: %w", err)
	}

	execPath := getExecutablePath()
	imageUUID, randomTTL := GenerateImageUUIDAndTTL()

	artefacts := &Artefacts{
		FBCTemplate: string(fbcYAML),
		Dockerfile:  GenerateCatalogDockerfile(),
		Makefile:    GenerateMakefile(g.bundleImage, execPath, imageUUID, randomTTL),
	}

	catalogYAML, err := g.renderCatalog(ctx, fbcTemplate)
	if err != nil {
		return artefacts, fmt.Errorf("rendering catalog: %w", err)
	}

	artefacts.CatalogYAML = catalogYAML
	return artefacts, nil
}

func getExecutablePath() string {
	execPath, err := os.Executable()
	if err != nil {
		return "bpfman-catalog"
	}
	return execPath
}

func (g *Generator) renderCatalog(ctx context.Context, fbcTemplate *FBCTemplate) (string, error) {
	if g.ompBinPath != "" {
		return RenderCatalogWithBinary(ctx, fbcTemplate, g.ompBinPath)
	}
	return RenderCatalog(ctx, fbcTemplate)
}
