package font

import (
	"os"
	"testing"

	"github.com/tdewolff/test"
)

func TestParseAFM(t *testing.T) {
	b, err := os.ReadFile("resources/pdfcorefonts/Helvetica.afm")
	test.Error(t, err)

	afm, err := ParseAFM(b)
	test.Error(t, err)

	test.T(t, afm.NumGlyphs(), uint16(315))
	test.T(t, afm.GlyphIndex('A'), uint16(33))
	test.T(t, afm.GlyphAdvance(33), uint16(667))

	test.T(t, afm.GlyphIndex('V'), uint16(54))
	test.T(t, afm.Kerning(33, 54), int16(-70))
}
