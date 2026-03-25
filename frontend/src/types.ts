// TypeScript types mirroring the Go models emitted via Wails events.

export interface ProcessingProgress {
  Stage: "splitting" | "rendering" | "ocr" | "grouping" | "saving";
  Current: number;
  Total: number;
  Description: string;
}

export interface Page {
  Number: number;
  PDFPath: string;
  ImagePath: string;
  OCRText: string;
}

export interface Student {
  FullName: string;
  Group: string;
  Pages: Page[];
}

export interface ParsedPage {
  Page: Page;
  FullName: string;
  Group: string;
  Confidence: number;
  IsOrphan: boolean;
}

export interface OutputFile {
  Student: Student;
  FilePath: string;
  FileName: string;
}

export interface ProcessingResult {
  OutputFiles: OutputFile[];
  Orphans: ParsedPage[];
  Errors: string[];
  AvgPageMs: number;
}
