package pipeline

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hellascape/scansplit/internal/models"
	"github.com/hellascape/scansplit/internal/pdf"
)

// saveStudent merges the student's pages into a single PDF and writes it to outputDir.
func (p *Pipeline) saveStudent(ctx context.Context, student models.Student, outputDir string) (models.OutputFile, error) {
	if len(student.Pages) == 0 {
		return models.OutputFile{}, fmt.Errorf("student %q has no pages", student.FullName)
	}

	fileName := safeFileName(student.FullName, student.Group) + ".pdf"
	outputPath := filepath.Join(outputDir, fileName)

	pagePaths := make([]string, len(student.Pages))
	for i, pg := range student.Pages {
		pagePaths[i] = pg.PDFPath
	}

	if err := pdf.MergePages(ctx, pagePaths, outputPath); err != nil {
		return models.OutputFile{}, fmt.Errorf("merge failed: %w", err)
	}

	p.logger.Info("saved student", "name", student.FullName, "pages", len(student.Pages), "path", outputPath)
	return models.OutputFile{
		Student:  student,
		FilePath: outputPath,
		FileName: fileName,
	}, nil
}

// saveOrphan saves a single unrecognised page as an individual PDF.
func (p *Pipeline) saveOrphan(ctx context.Context, page models.Page, outputDir string) (models.OutputFile, error) {
	fileName := fmt.Sprintf("неопознанная_стр_%02d.pdf", page.Number)
	outPath := filepath.Join(outputDir, fileName)

	if err := pdf.MergePages(ctx, []string{page.PDFPath}, outPath); err != nil {
		return models.OutputFile{}, err
	}

	p.logger.Info("saved orphan page", "page", page.Number, "path", outPath)
	return models.OutputFile{
		Student: models.Student{
			FullName: fmt.Sprintf("Неопознанная страница %d", page.Number),
			Pages:    []models.Page{page},
		},
		FilePath: outPath,
		FileName: fileName,
	}, nil
}

// safeFileName builds a filesystem-safe name: "Фамилия_Имя_Отчество_Группа".
// Characters illegal in Windows/macOS filenames are replaced with underscores.
func safeFileName(fullName, group string) string {
	raw := strings.Join(append(strings.Fields(fullName), group), "_")
	return strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		}
		return r
	}, raw)
}

