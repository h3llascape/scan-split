//go:build windows && amd64

package pdf

// Compatibility shim for precompiled MuPDF on MinGW GCC 15+.
//
// go-fitz ships libmupdf_windows_amd64.a compiled with an older MinGW GCC
// that exported __intrinsic_setjmpex as a runtime symbol. GCC 15 made it a
// pure compiler intrinsic — no exported symbol, hence the linker error.
//
// Fix: define __intrinsic_setjmpex as a tail-jump to _setjmpex (Windows CRT).
// A tail-jump (jmp, not call) does NOT push a new stack frame, so _setjmpex
// captures the correct context of the MuPDF caller — which is required for
// setjmp to work correctly.

/*
__asm__(
	".globl __intrinsic_setjmpex\n\t"
	"__intrinsic_setjmpex:\n\t"
	"jmp _setjmpex"
);
*/
import "C"
