package app

import (
	"gioui.org/font"
	"gioui.org/font/opentype"
)

func bundledFont(data []byte) []font.FontFace {
	faces, err := opentype.ParseCollection(data)
	if err != nil {
		panic(err)
	}
	return faces
}
