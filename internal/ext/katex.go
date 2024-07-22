package ext

import (
	_ "embed"
)

//go:embed wasm/renderkatex.wasm
var katexWasm []byte

var katexOptions = Options{
	CompileModule: compileFunc("katex", katexWasm, true),
	PoolSize:      8,
}

// StartKatex starts a new dispatcher for the Katex module.
func StartKatex() (Dispatcher[KatexInput, KatexOutput], error) {
	return Start[KatexInput, KatexOutput](katexOptions)
}

// See https://katex.org/docs/options.html
type KatexInput struct {
	ID          uint32 `json:"id"`
	Expression  string `json:"expression"`
	Output      string `json:"output"` // html, mathml, htmlAndMathml (default)
	DisplayMode bool   `json:"displayMode"`
}

type KatexOutput struct {
	ID     uint32 `json:"id"`
	Output string `json:"output"`
}

func (k KatexOutput) GetID() uint32 {
	return k.ID
}

func (k KatexInput) GetID() uint32 {
	return k.ID
}
