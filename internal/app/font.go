package app

import (
	_ "embed"

	"gioui.org/font"
	"gioui.org/font/opentype"
)

//go:embed fonts/JetBrainsMonoNL-Regular.ttf
var monoFont []byte

func bundledFont() []font.FontFace {
	faces, err := opentype.ParseCollection(monoFont)
	if err != nil {
		panic(err)
	}
	return faces
}
