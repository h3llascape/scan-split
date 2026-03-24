//go:build windows

package gosseract

// #cgo CXXFLAGS: -std=c++0x
// #cgo CPPFLAGS: -IC:/msys64/mingw64/include
// #cgo CPPFLAGS: -Wno-unused-result
// #cgo LDFLAGS: -LC:/msys64/mingw64/lib -lleptonica -ltesseract
import "C"
