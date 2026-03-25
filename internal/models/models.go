package models

// Page represents a single page extracted from the source PDF.
type Page struct {
	Number  int    // 1-based page number in the source PDF
	PDFPath string // Path to single-page PDF (temp file)
	OCRText string // Raw recognized text from OCR
}

// ParsedPage holds the OCR and parse results for a single page.
type ParsedPage struct {
	Page       Page
	FullName   string  // Extracted full name, e.g. "Иванов Иван Иванович"
	Group      string  // Extracted group code, e.g. "РИ-330942"
	Confidence float64 // Parsing confidence in range 0..1
	IsOrphan   bool    // True when name or group could not be extracted
}

// Student represents a student with all their recognized pages.
type Student struct {
	FullName string // "Иванов Иван Иванович"
	Group    string // "РИ-330942"
	Pages    []Page // Pages belonging to this student
}

// OutputFile describes a successfully generated output PDF.
type OutputFile struct {
	Student  Student
	FilePath string // Absolute path to the output PDF
	FileName string // Filename only, e.g. "Иванов_Иван_Иванович_РИ-330942.pdf"
}

// ProcessingResult is the final result returned after the pipeline completes.
type ProcessingResult struct {
	OutputFiles  []OutputFile // Successfully created student files
	Orphans      []ParsedPage // Pages that could not be attributed to any student
	Errors       []string     // Non-fatal errors collected during processing
	AvgPageMs    int64        // Average render+OCR time per page in milliseconds
}

// ProcessingProgress describes the current state of pipeline execution.
// Emitted via Wails event "processing:progress".
type ProcessingProgress struct {
	Stage       string // "splitting" | "rendering" | "ocr" | "grouping" | "saving"
	Current     int    // Current item index (1-based for display)
	Total       int    // Total items in this stage
	Description string // Human-readable description of the current step
}
