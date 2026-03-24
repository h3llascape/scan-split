# ScanSplit — build targets
# ─────────────────────────────────────────────────────────────────────────────
# macOS prereqs  : brew install tesseract leptonica
#                  go env -w CGO_CXXFLAGS="-I/opt/homebrew/include"
#                  go env -w CGO_LDFLAGS="-L/opt/homebrew/lib"
#
# Windows prereqs: MSYS2 MinGW64 shell
#                  pacman -S mingw-w64-x86_64-tesseract-ocr \
#                             mingw-w64-x86_64-leptonica
#
# NOTE: MuPDF is bundled inside go-fitz (no installation needed).
# NOTE: tessdata/rus.traineddata is embedded at build time.
#       Run `make tessdata` if the file is missing.
# ─────────────────────────────────────────────────────────────────────────────

WAILS       := $(shell go env GOPATH)/bin/wails
TESSDATA    := internal/ocr/tessdata/rus.traineddata
TESSDATA_URL := https://github.com/tesseract-ocr/tessdata_fast/raw/main/rus.traineddata

# macOS: Homebrew puts headers/libs here; Wails spawns sub-processes that
# don't inherit `go env -w` settings, so we export them explicitly.
export CGO_CXXFLAGS ?= -I/opt/homebrew/include
export CGO_LDFLAGS  ?= -L/opt/homebrew/lib

# ── Development ──────────────────────────────────────────────────────────────

.PHONY: dev
dev: check-tessdata
	$(WAILS) dev

# ── Tessdata ─────────────────────────────────────────────────────────────────

.PHONY: tessdata
tessdata: $(TESSDATA)

$(TESSDATA):
	@echo "Downloading Russian tessdata (fast model)..."
	@mkdir -p $(dir $@)
	curl -L --progress-bar -o $@ $(TESSDATA_URL)
	@echo "Done: $@"

.PHONY: check-tessdata
check-tessdata:
	@test -f $(TESSDATA) || \
		(echo "\nERROR: $(TESSDATA) not found.\nRun: make tessdata\n" && exit 1)

# ── macOS build & bundle ─────────────────────────────────────────────────────

.PHONY: build-mac
build-mac: check-tessdata
	$(WAILS) build -platform darwin/arm64

# Bundle libtesseract.dylib and its deps into the .app so it works without
# Tesseract installed on the end user's machine.
# Requires: brew install dylibbundler
.PHONY: bundle-mac
bundle-mac: build-mac
	@which dylibbundler > /dev/null || (echo "Run: brew install dylibbundler" && exit 1)
	@APP=build/bin/scansplit.app; \
	  BIN=$$APP/Contents/MacOS/scansplit; \
	  FWDIR=$$APP/Contents/Frameworks; \
	  mkdir -p $$FWDIR; \
	  dylibbundler -od -b -x $$BIN -d $$FWDIR -p @executable_path/../Frameworks; \
	  echo "Bundled. App is self-contained: $$APP"

# ── Windows build (run on Windows machine with MSYS2) ────────────────────────
# After building, copy required DLLs from MSYS2 to build/bin/.
# Adjust MSYS2_PREFIX if your MSYS2 is installed elsewhere.

MSYS2_PREFIX ?= C:/msys64/mingw64

.PHONY: build-win
build-win: check-tessdata
	$(WAILS) build -platform windows/amd64

.PHONY: bundle-win
bundle-win: build-win
	@echo "Copying Tesseract DLLs to build/bin/"
	@for dll in libtesseract-5.dll libleptonica-6.dll \
	             libgcc_s_seh-1.dll libstdc++-6.dll libwinpthread-1.dll \
	             libpng16-16.dll libjpeg-8.dll libtiff-6.dll libwebp-7.dll \
	             libarchive-13.dll libzstd.dll liblz4.dll liblzma-5.dll zlib1.dll; do \
	  cp -v "$(MSYS2_PREFIX)/bin/$$dll" build/bin/ 2>/dev/null || true; \
	done
	@echo "Done. Zip build/bin/ to distribute."

# ── Utility ──────────────────────────────────────────────────────────────────

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: clean
clean:
	rm -rf build/bin/
