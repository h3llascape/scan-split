package pdf

import (
	"fmt"
	"strconv"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	pdfmodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// ExtractPages extracts the specified 1-based page numbers from srcPath
// into a new PDF at outputPath, preserving the given order.
func ExtractPages(srcPath, outputPath string, pageNumbers []int) error {
	if len(pageNumbers) == 0 {
		return fmt.Errorf("no pages to extract")
	}

	sel := make([]string, len(pageNumbers))
	for i, n := range pageNumbers {
		sel[i] = strconv.Itoa(n)
	}

	conf := pdfmodel.NewDefaultConfiguration()
	conf.ValidationMode = pdfmodel.ValidationRelaxed

	if err := api.CollectFile(srcPath, outputPath, sel, conf); err != nil {
		return fmt.Errorf("failed to extract pages into %q: %w", outputPath, err)
	}
	return nil
}
