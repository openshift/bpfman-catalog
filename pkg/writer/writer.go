package writer

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/openshift/bpfman-catalog/pkg/manifests"
	"sigs.k8s.io/yaml"
)

// ManifestWriter writes manifests to files.
type ManifestWriter struct {
	outputDir string
}

// New creates a new manifest writer.
func New(outputDir string) *ManifestWriter {
	return &ManifestWriter{
		outputDir: outputDir,
	}
}

// WriteAll writes all manifests in a ManifestSet to files.
func (w *ManifestWriter) WriteAll(manifestSet *manifests.ManifestSet) error {
	return w.WriteAllSeparated(manifestSet)
}

// WriteAllSeparated writes manifests in separate subdirectories for
// flexible deployment.
//
// Creates:
//   - catalog/ - Namespace, IDMS, CatalogSource (catalog infrastructure)
//   - subscription/ - OperatorGroup, Subscription (operator installation)
func (w *ManifestWriter) WriteAllSeparated(manifestSet *manifests.ManifestSet) error {
	if err := os.MkdirAll(w.outputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	catalogDir := filepath.Join(w.outputDir, "catalog")
	subscriptionDir := filepath.Join(w.outputDir, "subscription")

	if err := os.MkdirAll(catalogDir, 0755); err != nil {
		return fmt.Errorf("creating catalog directory: %w", err)
	}

	if err := os.MkdirAll(subscriptionDir, 0755); err != nil {
		return fmt.Errorf("creating subscription directory: %w", err)
	}

	if manifestSet.Namespace != nil {
		if err := w.writeManifestToDir(catalogDir, "00-namespace.yaml", manifestSet.Namespace); err != nil {
			return fmt.Errorf("writing namespace: %w", err)
		}
	}

	if manifestSet.IDMS != nil {
		if err := w.writeManifestToDir(catalogDir, "01-idms.yaml", manifestSet.IDMS); err != nil {
			return fmt.Errorf("writing IDMS: %w", err)
		}
	}

	if manifestSet.CatalogSource != nil {
		if err := w.writeManifestToDir(catalogDir, "02-catalogsource.yaml", manifestSet.CatalogSource); err != nil {
			return fmt.Errorf("writing CatalogSource: %w", err)
		}
	}

	if manifestSet.OperatorGroup != nil {
		if err := w.writeManifestToDir(subscriptionDir, "03-operatorgroup.yaml", manifestSet.OperatorGroup); err != nil {
			return fmt.Errorf("writing OperatorGroup: %w", err)
		}
	}

	if manifestSet.Subscription != nil {
		if err := w.writeManifestToDir(subscriptionDir, "04-subscription.yaml", manifestSet.Subscription); err != nil {
			return fmt.Errorf("writing Subscription: %w", err)
		}
	}

	return nil
}

// writeManifestToDir writes a single manifest to a file in a specific
// directory.
func (w *ManifestWriter) writeManifestToDir(dir, filename string, manifest any) error {
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}

	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing file %s: %w", path, err)
	}

	return nil
}

// WriteSingle writes a single manifest to a specific file.
func (w *ManifestWriter) WriteSingle(filename string, manifest any) error {
	if err := os.MkdirAll(w.outputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	if data, ok := manifest.([]byte); ok {
		path := filepath.Join(w.outputDir, filename)
		if err := os.WriteFile(path, data, 0644); err != nil {
			return fmt.Errorf("writing file %s: %w", path, err)
		}
		return nil
	}

	return w.writeManifestToDir(w.outputDir, filename, manifest)
}
