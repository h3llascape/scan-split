package pdf

import (
	"context"
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	pdfmodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// MergePages merges the given single-page PDF files into a single output PDF at outputPath.
// Pages appear in the same order as inputPaths.
func MergePages(_ context.Context, inputPaths []string, outputPath string) error {
	if len(inputPaths) == 0 {
		return fmt.Errorf("no input pages to merge")
	}

	conf := pdfmodel.NewDefaultConfiguration()
	conf.ValidationMode = pdfmodel.ValidationRelaxed

	if err := api.MergeCreateFile(inputPaths, outputPath, false, conf); err != nil {
		return fmt.Errorf("failed to merge into %q: %w", outputPath, err)
	}

	return nil
}
