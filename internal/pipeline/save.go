package pipeline

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hellascape/scansplit/internal/models"
	"github.com/hellascape/scansplit/internal/pdf"
)

// saveStudent extracts the student's pages from the original PDF into outputDir.
func (p *Pipeline) saveStudent(ctx context.Context, student models.Student, outputDir string) (models.OutputFile, error) {
	if len(student.Pages) == 0 {
		return models.OutputFile{}, fmt.Errorf("student %q has no pages", student.FullName)
	}
	if ctx.Err() != nil {
		return models.OutputFile{}, ctx.Err()
	}

	fileName := safeFileName(student.FullName, student.Group) + ".pdf"
	outputPath := filepath.Join(outputDir, fileName)

	pageNums := make([]int, len(student.Pages))
	for i, pg := range student.Pages {
		pageNums[i] = pg.Number
	}

	if err := pdf.ExtractPages(student.Pages[0].SourcePath, outputPath, pageNums); err != nil {
		return models.OutputFile{}, fmt.Errorf("extract failed: %w", err)
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
	if ctx.Err() != nil {
		return models.OutputFile{}, ctx.Err()
	}

	fileName := fmt.Sprintf("неопознанная_стр_%02d.pdf", page.Number)
	outPath := filepath.Join(outputDir, fileName)

	if err := pdf.ExtractPages(page.SourcePath, outPath, []int{page.Number}); err != nil {
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

