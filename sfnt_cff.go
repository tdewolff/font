package font

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
)

// TODO: use FDSelect for Font DICTs
// TODO: CFF has winding rule even-odd? CFF2 has winding rule nonzero

type cffTable struct {
	version     int
	name        string
	top         *cffTopDICT
	globalSubrs *cffINDEX
	charStrings *cffINDEX
	fonts       *cffFontINDEX
}

func (sfnt *SFNT) parseCFF() error {
	b, ok := sfnt.Tables["CFF "]
	if !ok {
		return fmt.Errorf("CFF: missing table")
	}

	r := NewBinaryReader(b)
	major := r.ReadUint8()
	minor := r.ReadUint8()
	if major != 1 || minor != 0 {
		return fmt.Errorf("CFF: bad version")
	}
	hdrSize := r.ReadUint8()
	if hdrSize != 4 {
		return fmt.Errorf("CFF: bad hdrSize")
	}
	offSize := r.ReadUint8() // offSize, not actually used
	if offSize == 0 || 4 < offSize {
		return fmt.Errorf("CFF: bad offSize")
	}

	nameINDEX, err := parseINDEX(r, false)
	if err != nil {
		return fmt.Errorf("CFF: Name INDEX: %w", err)
	} else if len(nameINDEX.offset) != 2 {
		return fmt.Errorf("CFF: Name INDEX: bad count")
	}

	topINDEX, err := parseINDEX(r, false)
	if err != nil {
		return fmt.Errorf("CFF: Top INDEX: %w", err)
	} else if len(topINDEX.offset) != len(nameINDEX.offset) {
		return fmt.Errorf("CFF: Top INDEX: bad count")
	}

	stringINDEX, err := parseINDEX(r, false)
	if err != nil {
		return fmt.Errorf("CFF: String INDEX: %w", err)
	}

	topDICT, err := parseTopDICT(topINDEX.Get(0), stringINDEX)
	if err != nil {
		return fmt.Errorf("CFF: Top DICT: %w", err)
	} else if topDICT.CharstringType != 2 {
		return fmt.Errorf("CFF: Type %d Charstring format not supported", topDICT.CharstringType)
	}

	globalSubrsINDEX, err := parseINDEX(r, false)
	if err != nil {
		return fmt.Errorf("CFF: Global Subrs INDEX: %w", err)
	}

	r.Seek(uint32(topDICT.CharStrings))
	charStringsINDEX, err := parseINDEX(r, false)
	if err != nil {
		return fmt.Errorf("CFF: CharStrings INDEX: %w", err)
	}

	sfnt.CFF = &cffTable{
		version:     1,
		name:        string(nameINDEX.Get(0)),
		top:         topDICT,
		globalSubrs: globalSubrsINDEX,
		charStrings: charStringsINDEX,
	}

	if !topDICT.IsCID {
		if len(b) < topDICT.PrivateOffset || len(b)-topDICT.PrivateOffset < topDICT.PrivateLength {
			return fmt.Errorf("CFF: bad Private DICT offset")
		}
		privateDICT, err := parsePrivateDICT(b[topDICT.PrivateOffset:topDICT.PrivateOffset+topDICT.PrivateLength], false)
		if err != nil {
			return fmt.Errorf("CFF: Private DICT: %w", err)
		}

		localSubrsINDEX := &cffINDEX{}
		if privateDICT.Subrs != 0 {
			if len(b)-topDICT.PrivateOffset < privateDICT.Subrs {
				return fmt.Errorf("CFF: bad Local Subrs INDEX offset")
			}
			r.Seek(uint32(topDICT.PrivateOffset + privateDICT.Subrs))
			localSubrsINDEX, err = parseINDEX(r, false)
			if err != nil {
				return fmt.Errorf("CFF: Local Subrs INDEX: %w", err)
			}
		}
		sfnt.CFF.fonts = &cffFontINDEX{
			private:    []*cffPrivateDICT{privateDICT},
			localSubrs: []*cffINDEX{localSubrsINDEX},
			first:      []uint32{0, uint32(charStringsINDEX.Len())},
			fd:         []uint16{0},
		}
	} else {
		// CID font
		fonts, err := parseFontINDEX(b, topDICT.FDArray, topDICT.FDSelect, charStringsINDEX.Len(), false)
		if err != nil {
			return fmt.Errorf("CFF: %w", err)
		}
		sfnt.CFF.fonts = fonts
	}
	return nil
}

func (sfnt *SFNT) parseCFF2() error {
	return fmt.Errorf("CFF2: not supported")

	b, ok := sfnt.Tables["CFF2"]
	if !ok {
		return fmt.Errorf("CFF2: missing table")
	}

	r := NewBinaryReader(b)
	major := r.ReadUint8()
	minor := r.ReadUint8()
	if major != 2 || minor != 0 {
		return fmt.Errorf("CFF2: bad version")
	}
	headerSize := r.ReadUint8()
	if headerSize != 5 {
		return fmt.Errorf("CFF2: bad headerSize")
	}
	topDictLength := r.ReadUint16()

	topDICT, err := parseTopDICT2(r.ReadBytes(uint32(topDictLength)))
	if err != nil {
		return fmt.Errorf("CFF2: Top DICT: %w", err)
	}

	globalSubrsINDEX, err := parseINDEX(r, true)
	if err != nil {
		return fmt.Errorf("CFF2: Global Subrs INDEX: %w", err)
	}

	r.Seek(uint32(topDICT.CharStrings))
	charStringsINDEX, err := parseINDEX(r, true)
	if err != nil {
		return fmt.Errorf("CFF2: CharStrings INDEX: %w", err)
	}

	fonts, err := parseFontINDEX(b, topDICT.FDArray, topDICT.FDSelect, charStringsINDEX.Len(), true)
	if err != nil {
		return fmt.Errorf("CFF2: %w", err)
	}

	sfnt.CFF = &cffTable{
		version:     2,
		charStrings: charStringsINDEX,
		globalSubrs: globalSubrsINDEX,
		fonts:       fonts,
	}
	return nil
}

func (cff *cffTable) Version() int {
	return cff.version
}

func (cff *cffTable) TopDICT() *cffTopDICT {
	return cff.top
}

func (cff *cffTable) PrivateDICT(glyphID uint16) (*cffPrivateDICT, error) {
	return cff.fonts.GetPrivate(uint32(glyphID))
}

//type cffCharStringOp int32
//
//const (
//	cffHstem      cffCharStringOp = 1
//	cffVstem      cffCharStringOp = 3
//	cffVmoveto    cffCharStringOp = 4
//	cffRlineto    cffCharStringOp = 5
//	cffHlineto    cffCharStringOp = 6
//	cffVlineto    cffCharStringOp = 7
//	cffRrcurveto  cffCharStringOp = 8
//	cffCallsubr   cffCharStringOp = 10
//	cffReturn     cffCharStringOp = 11
//	cffEscape     cffCharStringOp = 12
//	cffEndchar    cffCharStringOp = 14
//	cffHstemhm    cffCharStringOp = 18
//	cffHintmask   cffCharStringOp = 19
//	cffCntrmask   cffCharStringOp = 20
//	cffRmoveto    cffCharStringOp = 21
//	cffHmoveto    cffCharStringOp = 22
//	cffVstemhm    cffCharStringOp = 23
//	cffRcurveline cffCharStringOp = 24
//	cffRlinecurve cffCharStringOp = 25
//	cffVvcurveto  cffCharStringOp = 26
//	cffHhcurveto  cffCharStringOp = 27
//	cffShortint   cffCharStringOp = 28
//	cffCallgsubr  cffCharStringOp = 29
//	cffVhcurveto  cffCharStringOp = 30
//	cffHvcurveto  cffCharStringOp = 31
//	cffAnd        cffCharStringOp = 256 + 3
//	cffOr         cffCharStringOp = 256 + 4
//	cffNot        cffCharStringOp = 256 + 5
//	cffAbs        cffCharStringOp = 256 + 9
//	cffAdd        cffCharStringOp = 256 + 10
//	cffSub        cffCharStringOp = 256 + 11
//	cffDiv        cffCharStringOp = 256 + 12
//	cffNeg        cffCharStringOp = 256 + 14
//	cffEq         cffCharStringOp = 256 + 15
//	cffDrop       cffCharStringOp = 256 + 18
//	cffPut        cffCharStringOp = 256 + 20
//	cffGet        cffCharStringOp = 256 + 21
//	cffIfelse     cffCharStringOp = 256 + 22
//	cffRandom     cffCharStringOp = 256 + 23
//	cffMul        cffCharStringOp = 256 + 24
//	cffSqrt       cffCharStringOp = 256 + 26
//	cffDup        cffCharStringOp = 256 + 27
//	cffExch       cffCharStringOp = 256 + 28
//	cffIndex      cffCharStringOp = 256 + 29
//	cffRoll       cffCharStringOp = 256 + 30
//	cffHflex      cffCharStringOp = 256 + 34
//	cffFlex       cffCharStringOp = 256 + 35
//	cffHflex1     cffCharStringOp = 256 + 36
//	cffFlex1      cffCharStringOp = 256 + 37
//)

func cffReadCharStringNumber(r *BinaryReader, b0 int32) int32 {
	var v int32
	if b0 == 28 {
		v = int32(r.ReadInt16()) << 16
	} else if b0 < 247 {
		v = (b0 - 139) << 16
	} else if b0 < 251 {
		b1 := int32(r.ReadUint8())
		v = ((b0-247)*256 + b1 + 108) << 16
	} else if b0 < 255 {
		b1 := int32(r.ReadUint8())
		v = (-(b0-251)*256 - b1 - 108) << 16
	} else {
		v = r.ReadInt32() // least-significant bits are fraction
	}
	return v
}

func (cff *cffTable) ToPath(p Pather, glyphID, ppem uint16, x0, y0, f float64, hinting Hinting) error {
	table := "CFF"
	if cff.version == 2 {
		table = "CFF2"
	}
	errBadNumOperands := fmt.Errorf("%v: bad number of operands for operator", table)

	charString := cff.charStrings.Get(glyphID)
	if charString == nil {
		return fmt.Errorf("%v: bad glyphID %v", table, glyphID)
	} else if 65525 < len(charString) {
		return fmt.Errorf("%v: charstring too long", table)
	}
	localSubrs, err := cff.fonts.GetLocalSubrs(uint32(glyphID))
	if err != nil {
		return fmt.Errorf("%v: %w", table, err)
	}

	// x,y are raised to most-significant 16 bits and treat less-significant bits as fraction
	var x, y int32
	f /= float64(1 << 16) // correct back

	hints := 0
	stack := []int32{} // TODO: may overflow?
	firstOperator := true
	callStack := []*BinaryReader{}
	r := NewBinaryReader(charString)
	for {
		if cff.version == 2 && r.Len() == 0 && 0 < len(callStack) {
			// end of subroutine
			r = callStack[len(callStack)-1]
			callStack = callStack[:len(callStack)-1]
			continue
		} else if r.Len() == 0 {
			break
		}

		b0 := int32(r.ReadUint8())
		if 32 <= b0 || b0 == 28 {
			v := cffReadCharStringNumber(r, b0)
			if cff.version == 1 && 48 <= len(stack) || cff.version == 2 && 513 <= len(stack) {
				return fmt.Errorf("%v: too many operands for operator", table)
			}
			stack = append(stack, v)
		} else {
			if firstOperator && cff.version == 1 && (b0 == 1 || b0 == 3 || b0 == 4 || b0 == 14 || 18 <= b0 && b0 <= 23) {
				// optionally parse width
				hasWidth := len(stack)%2 == 1
				if b0 == 22 || b0 == 4 {
					hasWidth = !hasWidth
				}
				if hasWidth {
					stack = stack[1:]
				}
			}
			if b0 != 29 && b0 != 10 && b0 != 11 {
				// callgsubr, callsubr, and return don't influence the width operator
				firstOperator = false
			}

			if b0 == 12 {
				b0 = 256 + int32(r.ReadUint8())
			}

			switch b0 {
			case 21:
				// rmoveto
				if len(stack) != 2 {
					return errBadNumOperands
				}
				x += stack[0]
				y += stack[1]
				p.Close()
				p.MoveTo(x0+f*float64(x), y0+f*float64(y))
				stack = stack[:0]
			case 22:
				// hmoveto
				if len(stack) != 1 {
					return errBadNumOperands
				}
				x += stack[0]
				p.Close()
				p.MoveTo(x0+f*float64(x), y0+f*float64(y))
				stack = stack[:0]
			case 4:
				// vmoveto
				if len(stack) != 1 {
					return errBadNumOperands
				}
				y += stack[0]
				p.Close()
				p.MoveTo(x0+f*float64(x), y0+f*float64(y))
				stack = stack[:0]
			case 5:
				// rlineto
				if len(stack) == 0 || len(stack)%2 != 0 {
					return errBadNumOperands
				}
				for i := 0; i < len(stack); i += 2 {
					x += stack[i+0]
					y += stack[i+1]
					p.LineTo(x0+f*float64(x), y0+f*float64(y))
				}
				stack = stack[:0]
			case 6, 7:
				// hlineto and vlineto
				if len(stack) == 0 {
					return errBadNumOperands
				}
				vertical := b0 == 7
				for i := 0; i < len(stack); i++ {
					if !vertical {
						x += stack[i]
					} else {
						y += stack[i]
					}
					p.LineTo(x0+f*float64(x), y0+f*float64(y))
					vertical = !vertical
				}
				stack = stack[:0]
			case 8:
				// rrcurveto
				if len(stack) == 0 || len(stack)%6 != 0 {
					return errBadNumOperands
				}
				for i := 0; i < len(stack); i += 6 {
					x += stack[i+0]
					y += stack[i+1]
					cpx1, cpy1 := x, y
					x += stack[i+2]
					y += stack[i+3]
					cpx2, cpy2 := x, y
					x += stack[i+4]
					y += stack[i+5]
					p.CubeTo(x0+f*float64(cpx1), y0+f*float64(cpy1), x0+f*float64(cpx2), y0+f*float64(cpy2), x0+f*float64(x), y0+f*float64(y))
				}
				stack = stack[:0]
			case 27, 26:
				// hhcurvetp and vvcurveto
				if len(stack) < 4 || len(stack)%4 != 0 && (len(stack)-1)%4 != 0 {
					return errBadNumOperands
				}
				vertical := b0 == 26
				i := 0
				if len(stack)%4 == 1 {
					if !vertical {
						y += stack[0]
					} else {
						x += stack[0]
					}
					i++
				}
				for ; i < len(stack); i += 4 {
					if !vertical {
						x += stack[i+0]
					} else {
						y += stack[i+0]
					}
					cpx1, cpy1 := x, y
					x += stack[i+1]
					y += stack[i+2]
					cpx2, cpy2 := x, y
					if !vertical {
						x += stack[i+3]
					} else {
						y += stack[i+3]
					}
					p.CubeTo(x0+f*float64(cpx1), y0+f*float64(cpy1), x0+f*float64(cpx2), y0+f*float64(cpy2), x0+f*float64(x), y0+f*float64(y))
				}
				stack = stack[:0]
			case 31, 30:
				// hvcurvetp and vhcurveto
				if len(stack) < 4 || len(stack)%4 != 0 && (len(stack)-1)%4 != 0 {
					return errBadNumOperands
				}
				vertical := b0 == 30
				for i := 0; i < len(stack); i += 4 {
					if !vertical {
						x += stack[i+0]
					} else {
						y += stack[i+0]
					}
					cpx1, cpy1 := x, y
					x += stack[i+1]
					y += stack[i+2]
					cpx2, cpy2 := x, y
					if !vertical {
						y += stack[i+3]
					} else {
						x += stack[i+3]
					}
					if i+5 == len(stack) {
						if !vertical {
							x += stack[i+4]
						} else {
							y += stack[i+4]
						}
						i++
					}
					p.CubeTo(x0+f*float64(cpx1), y0+f*float64(cpy1), x0+f*float64(cpx2), y0+f*float64(cpy2), x0+f*float64(x), y0+f*float64(y))
					vertical = !vertical
				}
				stack = stack[:0]
			case 24:
				// rcurveline
				if len(stack) < 2 || (len(stack)-2)%6 != 0 {
					return errBadNumOperands
				}
				i := 0
				for ; i < len(stack)-2; i += 6 {
					x += stack[i+0]
					y += stack[i+1]
					cpx1, cpy1 := x, y
					x += stack[i+2]
					y += stack[i+3]
					cpx2, cpy2 := x, y
					x += stack[i+4]
					y += stack[i+5]
					p.CubeTo(x0+f*float64(cpx1), y0+f*float64(cpy1), x0+f*float64(cpx2), y0+f*float64(cpy2), x0+f*float64(x), y0+f*float64(y))
				}
				x += stack[i+0]
				y += stack[i+1]
				p.LineTo(x0+f*float64(x), y0+f*float64(y))
				stack = stack[:0]
			case 25:
				// rlinecurve
				if len(stack) < 6 || (len(stack)-6)%2 != 0 {
					return errBadNumOperands
				}
				i := 0
				for ; i < len(stack)-6; i += 2 {
					x += stack[i+0]
					y += stack[i+1]
					p.LineTo(x0+f*float64(x), y0+f*float64(y))
				}
				x += stack[i+0]
				y += stack[i+1]
				cpx1, cpy1 := x, y
				x += stack[i+2]
				y += stack[i+3]
				cpx2, cpy2 := x, y
				x += stack[i+4]
				y += stack[i+5]
				p.CubeTo(x0+f*float64(cpx1), y0+f*float64(cpy1), x0+f*float64(cpx2), y0+f*float64(cpy2), x0+f*float64(x), y0+f*float64(y))
				stack = stack[:0]
			case 256 + 35:
				// flex
				if len(stack) != 13 {
					return errBadNumOperands
				}
				// always use cubic Béziers
				for i := 0; i < 12; i += 6 {
					x += stack[i+0]
					y += stack[i+1]
					cpx1, cpy1 := x, y
					x += stack[i+2]
					y += stack[i+3]
					cpx2, cpy2 := x, y
					x += stack[i+4]
					y += stack[i+5]
					p.CubeTo(x0+f*float64(cpx1), y0+f*float64(cpy1), x0+f*float64(cpx2), y0+f*float64(cpy2), x0+f*float64(x), y0+f*float64(y))
				}
				stack = stack[:0]
			case 256 + 34:
				// hflex
				if len(stack) != 7 {
					return errBadNumOperands
				}
				// always use cubic Béziers
				y1 := y
				x += stack[0]
				cpx1, cpy1 := x, y
				x += stack[1]
				y += stack[2]
				cpx2, cpy2 := x, y
				x += stack[3]
				p.CubeTo(x0+f*float64(cpx1), y0+f*float64(cpy1), x0+f*float64(cpx2), y0+f*float64(cpy2), x0+f*float64(x), y0+f*float64(y))

				x += stack[4]
				cpx1, cpy1 = x, y
				x += stack[5]
				y = y1
				cpx2, cpy2 = x, y
				x += stack[6]
				p.CubeTo(x0+f*float64(cpx1), y0+f*float64(cpy1), x0+f*float64(cpx2), y0+f*float64(cpy2), x0+f*float64(x), y0+f*float64(y))
				stack = stack[:0]
			case 256 + 36:
				// hflex1
				if len(stack) != 9 {
					return errBadNumOperands
				}
				// always use cubic Béziers
				y1 := y
				x += stack[0]
				y += stack[1]
				cpx1, cpy1 := x, y
				x += stack[2]
				y += stack[3]
				cpx2, cpy2 := x, y
				x += stack[4]
				p.CubeTo(x0+f*float64(cpx1), y0+f*float64(cpy1), x0+f*float64(cpx2), y0+f*float64(cpy2), x0+f*float64(x), y0+f*float64(y))

				x += stack[5]
				cpx1, cpy1 = x, y
				x += stack[6]
				y += stack[7]
				cpx2, cpy2 = x, y
				x += stack[8]
				y = y1
				p.CubeTo(x0+f*float64(cpx1), y0+f*float64(cpy1), x0+f*float64(cpx2), y0+f*float64(cpy2), x0+f*float64(x), y0+f*float64(y))
				stack = stack[:0]
			case 256 + 37:
				// flex1
				if len(stack) != 11 {
					return errBadNumOperands
				}
				// always use cubic Béziers
				x1, y1 := x, y
				x += stack[0]
				y += stack[1]
				cpx1, cpy1 := x, y
				x += stack[2]
				y += stack[3]
				cpx2, cpy2 := x, y
				x += stack[4]
				y += stack[5]
				p.CubeTo(x0+f*float64(cpx1), y0+f*float64(cpy1), x0+f*float64(cpx2), y0+f*float64(cpy2), x0+f*float64(x), y0+f*float64(y))

				x += stack[6]
				y += stack[7]
				cpx1, cpy1 = x, y
				x += stack[8]
				y += stack[9]
				cpx2, cpy2 = x, y
				dx, dy := x-x1, y-y1
				if dx < 0 {
					dx = -dx
				}
				if dy < 0 {
					dy = -dy
				}
				if dy < dx {
					x += stack[10]
					y = y1
				} else {
					x = x1
					y += stack[10]
				}
				p.CubeTo(x0+f*float64(cpx1), y0+f*float64(cpy1), x0+f*float64(cpx2), y0+f*float64(cpy2), x0+f*float64(x), y0+f*float64(y))
				stack = stack[:0]
			case 14:
				// endchar
				if cff.version == 2 {
					return fmt.Errorf("CFF2: unsupported operator %d", b0)
				} else if len(stack) == 4 {
					return fmt.Errorf("CFF: unsupported endchar operands")
				} else if len(stack) != 0 {
					return errBadNumOperands
				}
				p.Close()
				return nil
			case 1, 3, 18, 23:
				// hstem, vstem, hstemhm, vstemhm
				if len(stack) < 2 || len(stack)%2 != 0 {
					return errBadNumOperands
				}
				// hints are not used
				hints += len(stack) / 2
				if 96 < hints {
					return fmt.Errorf("%v: too many stem hints", table)
				}
				stack = stack[:0]
			case 19, 20:
				// hintmask, cntrmask
				if len(stack)%2 != 0 {
					return errBadNumOperands
				}
				if 0 < len(stack) {
					// vstem
					hints += len(stack) / 2
					if 96 < hints {
						return fmt.Errorf("%v: too many stem hints", table)
					}
					stack = stack[:0]
				}
				r := callStack[len(callStack)-1]
				r.ReadBytes(uint32((hints + 7) / 8))
			// TODO: arithmetic, storage, and conditional operators for CFF version 1?
			case 10, 29:
				// callsubr and callgsubr
				if 10 < len(callStack) {
					return fmt.Errorf("%v: too many nested subroutines", table)
				} else if len(stack) == 0 {
					return errBadNumOperands
				}

				n := 0
				if b0 == 10 {
					n = localSubrs.Len()
				} else {
					n = cff.globalSubrs.Len()
				}
				i := stack[len(stack)-1] >> 16
				if n < 1240 {
					i += 107
				} else if n < 33900 {
					i += 1131
				} else {
					i += 32768
				}
				stack = stack[:len(stack)-1]
				if i < 0 || math.MaxUint16 < i {
					return fmt.Errorf("%v: bad subroutine", table)
				}

				var subr []byte
				if b0 == 10 {
					subr = localSubrs.Get(uint16(i))
				} else {
					subr = cff.globalSubrs.Get(uint16(i))
				}
				if subr == nil {
					return fmt.Errorf("%v: bad subroutine", table)
				} else if 65525 < len(charString) {
					return fmt.Errorf("%v: subroutine too long", table)
				}
				callStack = append(callStack, r)
				r = NewBinaryReader(subr)
				firstOperator = true
			case 11:
				// return
				if cff.version == 2 {
					return fmt.Errorf("%v: unsupported operator %d", table, b0)
				} else if len(callStack) == 0 {
					return fmt.Errorf("%v: bad return", table)
				}
				r = callStack[len(callStack)-1]
				callStack = callStack[:len(callStack)-1]
			case 16:
				// blend
				if cff.version == 1 {
					return fmt.Errorf("CFF: unsupported operator %d", b0)
				}
				// TODO: blend
			case 15:
				// vsindex
				if cff.version == 1 {
					return fmt.Errorf("CFF: unsupported operator %d", b0)
				}
				// TODO: vsindex
			default:
				if 256 <= b0 {
					return fmt.Errorf("%v: unsupported operator 12 %d", table, b0-256)
				}
				return fmt.Errorf("%v: unsupported operator %d", table, b0)
			}
		}
	}

	if cff.version == 1 {
		return fmt.Errorf("CFF: charstring must end with endchar operator")
	}
	p.Close()
	return nil
}

func cffCharStringSubrsBias(n int) int {
	bias := 32768
	if n < 1240 {
		bias = 107
	} else if n < 33900 {
		bias = 1131
	}
	return bias
}

func (cff *cffTable) updateSubrs(index *cffINDEX, localSubrsMap, globalSubrsMap map[int32]int32) {
	k := 1                          // on index.offset
	var offset, shrunk uint32       // on index.data
	var posNumber, lenNumber uint32 // last number on stack (the subrs index)
	r := NewBinaryReader(index.data)
	for {
		if r.Len() == 0 {
			break
		}

		b0 := int32(r.ReadUint8())
		if b0 == 12 {
			b0 = 256 + int32(r.ReadUint8())
		}
		if 32 <= b0 || b0 == 28 {
			lenNumber = 1
			if b0 == 28 {
				lenNumber = 3
			} else if b0 == 255 {
				lenNumber = 5
			} else if 247 <= b0 {
				lenNumber = 2
			}
			r.ReadBytes(lenNumber - 1)
			posNumber = r.pos - lenNumber
		} else if b0 == 10 || b0 == 29 {
			if lenNumber == 0 {
				continue
			}

			// get last number (only works for valid charstrings)
			stack := index.data[posNumber : posNumber+lenNumber]
			num := cffReadCharStringNumber(NewBinaryReader(stack[1:]), int32(stack[0]))

			// get original index number
			n := 0
			if b0 == 10 {
				n = cff.fonts.localSubrs[0].Len()
			} else {
				n = cff.globalSubrs.Len()
			}
			i := num >> 16
			if n < 1240 {
				i += 107
			} else if n < 33900 {
				i += 1131
			} else {
				i += 32768
			}
			if i < 0 || math.MaxUint16 < i {
				continue
			}

			// create the new charString encoded subrs index number
			// the INDEX data never grows
			var j int32
			if b0 == 10 {
				j = localSubrsMap[i] // bias already applied
			} else if b0 == 29 {
				j = globalSubrsMap[i] // bias already applied
			}
			wNum := &BinaryWriter{index.data[posNumber:posNumber:len(index.data)]}
			if -107 <= j && j <= 107 {
				wNum.WriteUint8(uint8(j + 139))
			} else if 108 <= j && j <= 1131 {
				j -= 108
				wNum.WriteUint8(uint8(j/256 + 247))
				wNum.WriteUint8(uint8(j % 256))
			} else if -1131 <= j && j <= -108 {
				j = -j - 108
				wNum.WriteUint8(uint8(j/256 + 251))
				wNum.WriteUint8(uint8(j % 256))
			} else if -32768 <= j && j <= 32767 {
				wNum.WriteUint8(28)
				wNum.WriteUint16(uint16(j))
			} else {
				wNum.WriteUint8(255)
				wNum.WriteUint32(uint32(j << 16)) // is Fixed with 16-bit fraction
			}

			// update INDEX
			shrink := lenNumber - wNum.Len()
			if 0 < shrink {
				// update offsets
				end := posNumber + lenNumber - shrink
				for k < len(index.offset) && index.offset[k] < end {
					index.offset[k] -= shrunk
					k++
				}
				// move chunk forward
				if 0 < shrunk {
					copy(index.data[offset-shrunk:], index.data[offset:end])
				}
				shrunk += shrink
				offset = posNumber + lenNumber
			} else if shrink < 0 {
				panic("INDEX must not grow")
			}
		}
	}

	if 0 < shrunk {
		// update offsets
		for k < len(index.offset) {
			index.offset[k] -= shrunk
			k++
		}

		// move chunk forward
		copy(index.data[offset-shrunk:], index.data[offset:])
		index.data = index.data[:uint32(len(index.data))-shrunk]
	}
}

// reindex subroutines in the order in which they appear and rearrange the global and local subroutines INDEX
func (cff *cffTable) rearrangeSubrs() error {
	if 1 < len(cff.fonts.localSubrs) {
		return fmt.Errorf("must contain only one font")
	}

	table := "CFF"
	if cff.version == 2 {
		table = "CFF2"
	}

	// construct new Subrs INDEX
	localSubrs := &cffINDEX{}           // new INDEX
	globalSubrs := &cffINDEX{}          // new INDEX
	localSubrsMap := map[int32]int32{}  // old to new index
	globalSubrsMap := map[int32]int32{} // old to new index
	numGlyphID := uint16(cff.charStrings.Len())
	for glyphID := uint16(0); glyphID < numGlyphID; glyphID++ {
		charString := cff.charStrings.Get(glyphID)
		if charString == nil {
			return fmt.Errorf("%v: bad glyphID %v", table, glyphID)
		} else if 65525 < len(charString) {
			return fmt.Errorf("%v: charstring too long", table)
		}

		var posNumber, lenNumber uint32
		callStack := []*BinaryReader{}
		r := NewBinaryReader(charString)
		for {
			if cff.version == 2 && r.Len() == 0 && 0 < len(callStack) {
				// end of subroutine
				r = callStack[len(callStack)-1]
				callStack = callStack[:len(callStack)-1]
				continue
			} else if r.Len() == 0 {
				break
			}

			b0 := int32(r.ReadUint8())
			if b0 == 12 {
				b0 = 256 + int32(r.ReadUint8())
			}
			if 32 <= b0 || b0 == 28 {
				lenNumber = 1
				if b0 == 28 {
					lenNumber = 3
				} else if b0 == 255 {
					lenNumber = 5
				} else if 247 <= b0 {
					lenNumber = 2
				}
				r.ReadBytes(lenNumber - 1)
				posNumber = r.pos - lenNumber
			} else if b0 == 10 || b0 == 29 {
				// callsubrs and callgsubrs
				if 10 < len(callStack) {
					return fmt.Errorf("%v: too many nested subroutines", table)
				} else if lenNumber == 0 {
					return fmt.Errorf("%v: bad number of operands for operator", table)
				}

				// get last number (only works for valid charstrings)
				stack := r.buf[posNumber : posNumber+lenNumber]
				num := cffReadCharStringNumber(NewBinaryReader(stack[1:]), int32(stack[0]))

				n := 0
				if b0 == 10 {
					n = cff.fonts.localSubrs[0].Len()
				} else {
					n = cff.globalSubrs.Len()
				}
				i := num >> 16
				if n < 1240 {
					i += 107
				} else if n < 33900 {
					i += 1131
				} else {
					i += 32768
				}
				if i < 0 || math.MaxUint16 < i {
					return fmt.Errorf("%v: bad subroutine", table)
				}

				var subr []byte
				if b0 == 10 {
					subr = cff.fonts.localSubrs[0].Get(uint16(i))
					if _, ok := localSubrsMap[i]; !ok {
						localSubrsMap[i] = int32(localSubrs.Len())
						localSubrs.Add(subr) // copies data
					}
				} else {
					subr = cff.globalSubrs.Get(uint16(i))
					if _, ok := globalSubrsMap[i]; !ok {
						globalSubrsMap[i] = int32(globalSubrs.Len())
						globalSubrs.Add(subr) // copies data
					}
				}

				if subr == nil {
					return fmt.Errorf("%v: bad subroutine", table)
				} else if 65525 < len(charString) {
					return fmt.Errorf("%v: subroutine too long", table)
				}
				callStack = append(callStack, r)
				r = NewBinaryReader(subr)
			} else if b0 == 11 {
				// return
				if cff.version == 2 {
					return fmt.Errorf("%v: unsupported operator %d", table, b0)
				} else if len(callStack) == 0 {
					return fmt.Errorf("%v: bad return", table)
				}
				r = callStack[len(callStack)-1]
				callStack = callStack[:len(callStack)-1]
			}
		}
	}

	// update new indices with bias
	localBias := int32(cffCharStringSubrsBias(localSubrs.Len()))
	for i, j := range localSubrsMap {
		localSubrsMap[i] = j - localBias
	}
	globalBias := int32(cffCharStringSubrsBias(globalSubrs.Len()))
	for i, j := range globalSubrsMap {
		globalSubrsMap[i] = j - globalBias
	}

	// update subrs indices in charstrings
	cff.updateSubrs(cff.charStrings, localSubrsMap, globalSubrsMap)
	cff.updateSubrs(localSubrs, localSubrsMap, globalSubrsMap)
	cff.updateSubrs(globalSubrs, localSubrsMap, globalSubrsMap)

	// set new Subrs INDEX
	cff.globalSubrs = globalSubrs
	if 0 < len(cff.fonts.localSubrs) {
		cff.fonts.localSubrs = []*cffINDEX{localSubrs}
	}
	return nil
}

type cffINDEX struct {
	offset []uint32
	data   []byte
}

func (t *cffINDEX) Len() int {
	if len(t.offset) == 0 {
		return 0
	}
	return len(t.offset) - 1
}

func (t *cffINDEX) Get(i uint16) []byte {
	if int(i) < t.Len() {
		return t.data[t.offset[i]:t.offset[i+1]]
	}
	return nil
}

func (t *cffINDEX) GetSID(sid int) string {
	// only for String INDEX
	if sid < len(cffStandardStrings) {
		return cffStandardStrings[sid]
	}
	sid -= len(cffStandardStrings)
	if math.MaxUint16 < sid {
		return ""
	}
	if b := t.Get(uint16(sid)); b != nil {
		return string(b)
	}
	return ""
}

func (t *cffINDEX) Add(data []byte) int {
	if len(t.offset) == 0 {
		t.offset = append(t.offset, 0)
	}
	t.data = append(t.data, data...)
	t.offset = append(t.offset, uint32(len(t.data)))
	return len(t.offset) - 2
}

func (t *cffINDEX) AddSID(data []byte) int {
	for i, s := range cffStandardStrings {
		if bytes.Equal(data, []byte(s)) {
			return i
		}
	}
	for i := 0; i+1 < len(t.offset); i++ {
		if bytes.Equal(data, t.data[t.offset[i]:t.offset[i+1]]) {
			return i
		}
	}
	return t.Add(data) + len(cffStandardStrings)
}

func parseINDEX(r *BinaryReader, isCFF2 bool) (*cffINDEX, error) {
	t := &cffINDEX{}
	var count uint32
	if !isCFF2 {
		count = uint32(r.ReadUint16())
	} else {
		count = r.ReadUint32()
	}
	if count == 0 {
		// empty
		return t, nil
	} else if 1e6 < count {
		return nil, fmt.Errorf("too big")
	}

	offSize := r.ReadUint8()
	if offSize == 0 || 4 < offSize {
		return nil, fmt.Errorf("bad offSize")
	}
	if r.Len() < uint32(offSize)*(uint32(count)+1) {
		return nil, fmt.Errorf("bad data")
	}

	t.offset = make([]uint32, count+1)
	if offSize == 1 {
		for i := uint32(0); i < count+1; i++ {
			t.offset[i] = uint32(r.ReadUint8()) - 1
		}
	} else if offSize == 2 {
		for i := uint32(0); i < count+1; i++ {
			t.offset[i] = uint32(r.ReadUint16()) - 1
		}
	} else if offSize == 3 {
		for i := uint32(0); i < count+1; i++ {
			t.offset[i] = uint32(r.ReadUint16())<<8 + uint32(r.ReadUint8()) - 1
		}
	} else {
		for i := uint32(0); i < count+1; i++ {
			t.offset[i] = r.ReadUint32() - 1
		}
	}
	if r.Len() < t.offset[count] {
		return nil, fmt.Errorf("bad data")
	}
	t.data = r.ReadBytes(t.offset[count])
	return t, nil
}

func cffINDEXOffSize(n int) int {
	if n <= math.MaxUint8 {
		return 1
	} else if n <= math.MaxUint16 {
		return 2
	} else if n <= (2<<24 - 1) {
		return 3
	}
	return 4
}

func (t *cffINDEX) Write() ([]byte, error) {
	if math.MaxUint16 < len(t.offset)-1 {
		return nil, fmt.Errorf("too many indices")
	} else if len(t.data) == 0 {
		return []byte{0, 0}, nil // zero count
	} else if len(t.offset) == 0 || t.offset[0] != 0 || int(t.offset[len(t.offset)-1]) != len(t.data) {
		return nil, fmt.Errorf("bad offsets")
	}

	offSize := cffINDEXOffSize(len(t.data) + 1)
	n := 3 + len(t.data) + offSize*len(t.offset)
	w := NewBinaryWriter(make([]byte, 0, n))
	w.WriteUint16(uint16(len(t.offset) - 1))
	w.WriteUint8(uint8(offSize))
	if offSize == 1 {
		for _, offset := range t.offset {
			w.WriteUint8(uint8(offset) + 1)
		}
	} else if offSize == 2 {
		for _, offset := range t.offset {
			w.WriteUint16(uint16(offset) + 1)
		}
	} else if offSize == 3 {
		for _, offset := range t.offset {
			w.WriteUint8(uint8((offset + 1) >> 16))
			w.WriteUint16(uint16((offset + 1) & 0x00FF))
		}
	} else if offSize == 4 {
		for _, offset := range t.offset {
			w.WriteUint32(offset + 1)
		}
	}
	w.WriteBytes(t.data)
	return w.Bytes(), nil
}

type cffTopDICT struct {
	IsSynthetic bool
	IsCID       bool

	Version            string
	Notice             string
	Copyright          string
	FullName           string
	FamilyName         string
	Weight             string
	IsFixedPitch       bool
	ItalicAngle        float64
	UnderlinePosition  float64
	UnderlineThickness float64
	PaintType          int
	CharstringType     int
	FontMatrix         [6]float64
	UniqueID           int
	FontBBox           [4]float64
	StrokeWidth        float64
	XUID               []int
	Charset            int
	Encoding           int
	CharStrings        int
	PrivateOffset      int
	PrivateLength      int
	SyntheticBase      int
	PostScript         string
	BaseFontName       string
	BaseFontBlend      []int
	ROS1               string
	ROS2               string
	ROS3               int
	CIDFontVersion     int
	CIDFontRevision    int
	CIDFontType        int
	CIDCount           int
	UIDBase            int
	FDArray            int
	FDSelect           int
	FontName           string
	Vstore             int // CFF2
}

func parseTopDICT(b []byte, stringINDEX *cffINDEX) (*cffTopDICT, error) {
	dict := &cffTopDICT{
		UnderlinePosition:  -100,
		UnderlineThickness: 50,
		CharstringType:     2,
		FontMatrix:         [6]float64{0.001, 0.0, 0.0, 0.001, 0.0, 0.0},
		CIDCount:           8720,
	}
	err := parseDICT(b, false, func(b0 int, is []int, fs []float64) bool {
		switch b0 {
		case 0:
			dict.Version = stringINDEX.GetSID(is[0])
		case 1:
			dict.Notice = stringINDEX.GetSID(is[0])
		case 256 + 0:
			dict.Copyright = stringINDEX.GetSID(is[0])
		case 2:
			dict.FullName = stringINDEX.GetSID(is[0])
		case 3:
			dict.FamilyName = stringINDEX.GetSID(is[0])
		case 4:
			dict.Weight = stringINDEX.GetSID(is[0])
		case 256 + 1:
			dict.IsFixedPitch = is[0] != 0
		case 256 + 2:
			dict.ItalicAngle = fs[0]
		case 256 + 3:
			dict.UnderlinePosition = fs[0]
		case 256 + 4:
			dict.UnderlineThickness = fs[0]
		case 256 + 5:
			dict.PaintType = is[0]
		case 256 + 6:
			dict.CharstringType = is[0]
		case 256 + 7:
			copy(dict.FontMatrix[:], fs)
		case 13:
			dict.UniqueID = is[0]
		case 5:
			copy(dict.FontBBox[:], fs)
		case 256 + 8:
			dict.StrokeWidth = fs[0]
		case 14:
			dict.XUID = is
		case 15:
			dict.Charset = is[0]
		case 16:
			dict.Encoding = is[0]
		case 17:
			dict.CharStrings = is[0]
		case 18:
			dict.PrivateOffset = is[1]
			dict.PrivateLength = is[0]
		case 256 + 20:
			dict.IsSynthetic = true
			dict.SyntheticBase = is[0]
		case 256 + 21:
			dict.PostScript = stringINDEX.GetSID(is[0])
		case 256 + 22:
			dict.BaseFontName = stringINDEX.GetSID(is[0])
		case 256 + 23:
			dict.BaseFontBlend = is
		case 256 + 30:
			// TODO: it is unclear how the ROS operator influences the GIDs/CIDs
			dict.IsCID = true
			dict.ROS1 = stringINDEX.GetSID(is[0])
			dict.ROS2 = stringINDEX.GetSID(is[1])
			dict.ROS3 = is[2]
		case 256 + 31:
			dict.CIDFontVersion = is[0]
		case 256 + 32:
			dict.CIDFontRevision = is[0]
		case 256 + 33:
			dict.CIDFontType = is[0]
		case 256 + 34:
			dict.CIDCount = is[0]
		case 256 + 35:
			dict.UIDBase = is[0]
		case 256 + 36:
			dict.FDArray = is[0]
		case 256 + 37:
			dict.FDSelect = is[0]
		case 256 + 38:
			dict.FontName = stringINDEX.GetSID(is[0])
		default:
			return false
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	return dict, nil
}

func (t *cffTopDICT) Write(strings *cffINDEX) ([]byte, error) {
	// TODO: some values have no default and may need to be written always
	w := NewBinaryWriter([]byte{})
	if t.Version != "" {
		writeDICTEntry(w, 0, strings.AddSID([]byte(t.Version)))
	}
	if t.Notice != "" {
		writeDICTEntry(w, 1, strings.AddSID([]byte(t.Notice)))
	}
	if t.Copyright != "" {
		writeDICTEntry(w, 256+0, strings.AddSID([]byte(t.Copyright)))
	}
	if t.FullName != "" {
		writeDICTEntry(w, 2, strings.AddSID([]byte(t.FullName)))
	}
	if t.FamilyName != "" {
		writeDICTEntry(w, 3, strings.AddSID([]byte(t.FamilyName)))
	}
	if t.Weight != "" {
		writeDICTEntry(w, 4, strings.AddSID([]byte(t.Weight)))
	}
	if t.IsFixedPitch {
		writeDICTEntry(w, 256+1, 1)
	}
	if t.ItalicAngle != 0.0 {
		writeDICTEntry(w, 256+2, t.ItalicAngle)
	}
	if t.UnderlinePosition != -100.0 {
		writeDICTEntry(w, 256+3, t.UnderlinePosition)
	}
	if t.UnderlineThickness != 50.0 {
		writeDICTEntry(w, 256+4, t.UnderlineThickness)
	}
	if t.PaintType != 0 {
		writeDICTEntry(w, 256+5, t.PaintType)
	}
	if t.CharstringType != 2 {
		return nil, fmt.Errorf("CharstringType must be 2")
	}
	if t.FontMatrix != [6]float64{0.001, 0, 0, 0.001, 0, 0} {
		writeDICTEntry(w, 256+7, t.FontMatrix[0], t.FontMatrix[1], t.FontMatrix[2], t.FontMatrix[3], t.FontMatrix[4], t.FontMatrix[5])
	}
	if t.UniqueID != 0 {
		writeDICTEntry(w, 13, t.UniqueID)
	}
	if t.FontBBox != [4]float64{0, 0, 0, 0} {
		writeDICTEntry(w, 5, t.FontBBox[0], t.FontBBox[1], t.FontBBox[2], t.FontBBox[3])
	}
	if t.StrokeWidth != 0.0 {
		writeDICTEntry(w, 256+8, t.StrokeWidth)
	}
	if 0 < len(t.XUID) {
		writeDICTEntry(w, 14, t.XUID)
	}
	// not supported
	//if t.Charset != 0 {
	//	writeDICTEntry(w, 15, t.Charset)
	//}
	//if t.Encoding != 0 {
	//	writeDICTEntry(w, 16, t.Encoding)
	//}
	// set later
	//if t.CharStrings != 0 {
	//	writeDICTEntry(w, 17, t.CharStrings)
	//}
	//if t.Private != 0 {
	//	writeDICTEntry(w, 18, t.Private)
	//}
	if t.SyntheticBase != 0 {
		writeDICTEntry(w, 256+20, t.SyntheticBase)
	}
	if t.PostScript != "" {
		writeDICTEntry(w, 256+21, strings.AddSID([]byte(t.PostScript)))
	}
	if t.BaseFontName != "" {
		writeDICTEntry(w, 256+22, strings.AddSID([]byte(t.BaseFontName)))
	}
	if 0 < len(t.BaseFontBlend) {
		writeDICTEntry(w, 256+23, t.BaseFontBlend)
	}
	return w.Bytes(), nil
}

type cffFontDICT struct {
	PrivateOffset int
	PrivateLength int
}

func parseFontDICT(b []byte, isCFF2 bool) (*cffFontDICT, error) {
	dict := &cffFontDICT{}
	return dict, parseDICT(b, isCFF2, func(b0 int, is []int, fs []float64) bool {
		switch b0 {
		case 18:
			dict.PrivateOffset = is[1]
			dict.PrivateLength = is[0]
		case 256 + 7:
			// FontMatrix
		case 256 + 38:
			// FontName
		default:
			return false
		}
		return true
	})
}

type cffPrivateDICT struct {
	BlueValues        []float64
	OtherBlues        []float64
	FamilyBlues       []float64
	FamilyOtherBlues  []float64
	BlueScale         float64
	BlueShift         float64
	BlueFuzz          float64
	StdHW             float64
	StdVW             float64
	StemSnapH         []float64
	StemSnapV         []float64
	ForceBold         bool
	LanguageGroup     int
	ExpansionFactor   float64
	InitialRandomSeed int
	Subrs             int
	DefaultWidthX     float64
	NominalWidthX     float64

	// CFF2
	Vsindex int
	Blend   []float64
}

func parsePrivateDICT(b []byte, isCFF2 bool) (*cffPrivateDICT, error) {
	dict := &cffPrivateDICT{
		BlueScale:       0.039625,
		BlueShift:       7.0,
		BlueFuzz:        1.0,
		ExpansionFactor: 0.06,
	}

	return dict, parseDICT(b, isCFF2, func(b0 int, is []int, fs []float64) bool {
		switch b0 {
		case 6:
			dict.BlueValues = fs
		case 7:
			dict.OtherBlues = fs
		case 8:
			dict.FamilyBlues = fs
		case 9:
			dict.FamilyOtherBlues = fs
		case 256 + 9:
			dict.BlueScale = fs[0]
		case 256 + 10:
			dict.BlueShift = fs[0]
		case 256 + 11:
			dict.BlueFuzz = fs[0]
		case 10:
			dict.StdHW = fs[0]
		case 11:
			dict.StdVW = fs[0]
		case 256 + 12:
			dict.StemSnapH = fs
		case 256 + 13:
			dict.StemSnapV = fs
		case 256 + 14:
			dict.ForceBold = is[0] != 0
		case 256 + 17:
			dict.LanguageGroup = is[0]
		case 256 + 18:
			dict.ExpansionFactor = fs[0]
		case 256 + 19:
			dict.InitialRandomSeed = is[0]
		case 19:
			dict.Subrs = is[0]
		case 20:
			dict.DefaultWidthX = fs[0]
		case 21:
			dict.NominalWidthX = fs[0]
		case 22:
			dict.Vsindex = is[0]
		case 23:
			dict.Blend = fs
		default:
			return false
		}
		return true
	})
}

func (t *cffPrivateDICT) Write() ([]byte, error) {
	// TODO: some values have no default and may need to be written always
	w := NewBinaryWriter([]byte{})
	if 0 < len(t.BlueValues) {
		writeDICTEntry(w, 6, t.BlueValues)
	}
	if 0 < len(t.OtherBlues) {
		writeDICTEntry(w, 7, t.OtherBlues)
	}
	if 0 < len(t.FamilyBlues) {
		writeDICTEntry(w, 8, t.FamilyBlues)
	}
	if 0 < len(t.FamilyOtherBlues) {
		writeDICTEntry(w, 9, t.FamilyOtherBlues)
	}
	if t.BlueScale != 0.039625 {
		writeDICTEntry(w, 256+9, t.BlueScale)
	}
	if t.BlueShift != 7.0 {
		writeDICTEntry(w, 256+10, t.BlueShift)
	}
	if t.BlueFuzz != 1.0 {
		writeDICTEntry(w, 256+11, t.BlueFuzz)
	}
	if t.StdHW != 0.0 {
		writeDICTEntry(w, 10, t.StdHW)
	}
	if t.StdVW != 0.0 {
		writeDICTEntry(w, 11, t.StdVW)
	}
	if len(t.StemSnapH) != 0 {
		writeDICTEntry(w, 256+12, t.StemSnapH)
	}
	if len(t.StemSnapV) != 0 {
		writeDICTEntry(w, 256+13, t.StemSnapV)
	}
	if t.ForceBold {
		writeDICTEntry(w, 256+14, t.ForceBold)
	}
	if t.LanguageGroup != 0 {
		writeDICTEntry(w, 256+17, t.LanguageGroup)
	}
	if t.ExpansionFactor != 0.06 {
		writeDICTEntry(w, 256+18, t.ExpansionFactor)
	}
	if t.InitialRandomSeed != 0 {
		writeDICTEntry(w, 256+19, t.InitialRandomSeed)
	}
	//if t.Subrs != 0 {
	//	return nil, fmt.Errorf("Local Subrs not supported")
	//}
	if t.DefaultWidthX != 0 {
		writeDICTEntry(w, 20, t.DefaultWidthX)
	}
	if t.NominalWidthX != 0 {
		writeDICTEntry(w, 21, t.NominalWidthX)
	}
	return w.Bytes(), nil
}

func parseTopDICT2(b []byte) (*cffTopDICT, error) {
	dict := &cffTopDICT{
		FontMatrix: [6]float64{0.001, 0.0, 0.0, 0.001, 0.0, 0.0},
	}
	return dict, parseDICT(b, true, func(b0 int, is []int, fs []float64) bool {
		switch b0 {
		case 256 + 7:
			copy(dict.FontMatrix[:], fs)
		case 17:
			dict.CharStrings = is[0]
		case 256 + 36:
			dict.FDArray = is[0]
		case 256 + 37:
			dict.FDSelect = is[0]
		case 24:
			dict.Vstore = is[0]
		default:
			return false
		}
		return true
	})
}

func parseDICT(b []byte, isCFF2 bool, callback func(b0 int, is []int, fs []float64) bool) error {
	opSize := map[int]int{
		256 + 7:  6,
		5:        4,
		14:       -1,
		18:       2,
		256 + 23: -1,
		256 + 30: 3,
		6:        -1,
		7:        -1,
		8:        -1,
		9:        -1,
		256 + 12: -1,
		256 + 13: -1,
	}

	r := NewBinaryReader(b)
	ints := []int{}
	reals := []float64{}
	for 0 < r.Len() {
		b0 := int(r.ReadUint8())
		if b0 < 22 {
			// operator
			if b0 == 12 {
				b0 = 256 + int(r.ReadUint8())
			}

			size := 1
			if s, ok := opSize[b0]; ok {
				if s == -1 {
					size = len(ints)
				} else {
					size = s
				}
			}
			if len(ints) < size {
				return fmt.Errorf("too few operands for operator")
			}

			is := ints[len(ints)-size:]
			fs := reals[len(reals)-size:]
			ints = ints[:len(ints)-size]
			reals = reals[:len(reals)-size]

			if ok := callback(b0, is, fs); !ok {
				return fmt.Errorf("bad operator")
			}
		} else if 22 <= b0 && b0 < 28 || b0 == 31 || b0 == 255 {
			// reserved
		} else {
			if !isCFF2 && 48 <= len(ints) || isCFF2 && 513 <= len(ints) {
				return fmt.Errorf("too many operands for operator")
			}
			i, f := parseDICTNumber(b0, r)
			if math.IsNaN(f) {
				f = float64(i)
			} else {
				i = int(f + 0.5)
			}
			ints = append(ints, i)
			reals = append(reals, f)
		}
	}
	return nil
}

func parseDICTNumber(b0 int, r *BinaryReader) (int, float64) {
	if b0 < 28 {
		// operator
		return 0, math.NaN()
	} else if b0 == 28 {
		return int(r.ReadInt16()), math.NaN()
	} else if b0 == 29 {
		return int(r.ReadInt32()), math.NaN()
	} else if b0 == 30 {
		num := []byte{}
		for {
			b := r.ReadUint8()
			for i := 0; i < 2; i++ {
				switch b >> 4 {
				case 0x0A:
					num = append(num, '.')
				case 0x0B:
					num = append(num, 'E')
				case 0x0C:
					num = append(num, 'E', '-')
				case 0x0D:
					// reserved
				case 0x0E:
					num = append(num, '-')
				case 0x0F:
					f, err := strconv.ParseFloat(string(num), 32)
					if err != nil {
						return 0, math.NaN()
					}
					return 0, f
				default:
					num = append(num, '0'+byte(b>>4))
				}
				b = b << 4
			}
		}
	} else if b0 == 31 {
		// reserved
		return 0, math.NaN()
	} else if b0 < 247 {
		return b0 - 139, math.NaN()
	} else if b0 < 251 {
		b1 := int(r.ReadUint8())
		return (b0-247)*256 + b1 + 108, math.NaN()
	} else if b0 < 255 {
		b1 := int(r.ReadUint8())
		return -(b0-251)*256 - b1 - 108, math.NaN()
	}
	// reserved
	return 0, math.NaN()
}

// returns: integer, float, useFloat, ok
func cffDICTNumber(val any) (int, float64, bool, bool) {
	switch v := val.(type) {
	case int:
		return v, float64(v), false, true
	case float64:
		var i int
		useFloat := false
		if integer, frac := math.Modf(v); frac == 0.0 {
			i = int(integer + 0.5)
		} else {
			useFloat = true
		}
		return i, v, useFloat, true
	default:
		return 0, 0.0, false, false
	}
}

func cffDICTFloat(f float64) ([]byte, int) {
	floatNibbles := strconv.AppendFloat([]byte{}, f, 'G', 6, 64)
	n := int(math.Ceil(float64(len(floatNibbles)+1)/2.0) + 0.5) // includes end nibbles
	return floatNibbles, 1 + n                                  // key and value
}

func cffDICTNumberSize(v any) int {
	i, f, useFloat, ok := cffDICTNumber(v)
	if !ok {
		return 0
	}
	_, nFloat := cffDICTFloat(f)
	if useFloat {
		return nFloat
	}
	nInt := cffDICTIntegerSize(i)
	if nFloat < nInt {
		return nFloat
	}
	return nInt
}

func cffDICTIntegerSize(i int) int {
	if -107 <= i && i <= 107 {
		return 1
	} else if -1131 <= i && i <= -108 || 108 <= i && i <= 1131 {
		return 2
	} else if -32767 <= i && i <= 32767 {
		return 3
	}
	return 5
}

func writeDICTEntry(w *BinaryWriter, op int, vals ...any) error {
	if len(vals) == 1 {
		switch vs := vals[0].(type) {
		case []int:
			vals = make([]any, 0, len(vs))
			for _, v := range vs {
				vals = append(vals, v)
			}
		case []float64:
			vals = make([]any, 0, len(vs))
			for _, v := range vs {
				vals = append(vals, v)
			}
		}
	}
	if 48 < len(vals) {
		return fmt.Errorf("too many operands")
	}

	for _, val := range vals {
		i, f, useFloat, ok := cffDICTNumber(val)
		if !ok {
			return fmt.Errorf("unknown operand type: %T", val)
		}

		floatNibbles, nFloat := cffDICTFloat(f)
		if useFloat || nFloat < cffDICTIntegerSize(i) {
			n := 0
			var b uint8
			w.WriteUint8(30)
			for i := 0; i < len(floatNibbles); i++ {
				b <<= 4
				switch floatNibbles[i] {
				case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
					b |= uint8(floatNibbles[i] - '0')
				case '.':
					b |= 0x0A
				case 'E':
					if i+1 < len(floatNibbles) && floatNibbles[i+1] == '-' {
						b |= 0x0C
						i++
					} else {
						b |= 0x0B
					}
				case '-':
					b |= 0x0E
				}
				n++
				if n%2 == 0 {
					w.WriteUint8(b)
					b = 0
				}
			}
			if n%2 == 1 {
				b <<= 4
				b |= 0x0F
				w.WriteUint8(b)
			} else {
				w.WriteUint8(0xFF)
			}
		} else if -107 <= i && i <= 107 {
			w.WriteUint8(uint8(i + 139))
		} else if 108 <= i && i <= 1131 {
			i -= 108
			w.WriteUint8(uint8(i/256 + 247))
			w.WriteUint8(uint8(i % 256))
		} else if -1131 <= i && i <= -108 {
			i = -i - 108
			w.WriteUint8(uint8(i/256 + 251))
			w.WriteUint8(uint8(i % 256))
		} else if -32768 <= i && i <= 32767 {
			w.WriteUint8(28)
			w.WriteUint16(uint16(i))
		} else {
			w.WriteUint8(29)
			w.WriteUint32(uint32(i))
		}
	}

	if 256+256 <= op || op == 12 {
		return fmt.Errorf("bad operator: %v", op)
	} else if 256 <= op {
		w.WriteUint8(0x0C)
		op -= 256
	}
	w.WriteUint8(uint8(op))
	return nil
}

type cffFontINDEX struct {
	private    []*cffPrivateDICT
	localSubrs []*cffINDEX

	fds   []uint8 // fds or the other two are used
	first []uint32
	fd    []uint16
}

func (t *cffFontINDEX) Index(glyphID uint32) (uint16, bool) {
	if t.fds != nil {
		if len(t.fds) <= int(glyphID) {
			return 0, false
		}
		return uint16(t.fds[glyphID]), true
	} else if t.first[len(t.first)-1] <= glyphID {
		return 0, false
	}

	i := 0
	for t.first[i+1] <= glyphID {
		i++
	}
	return t.fd[i], true
}

func (t *cffFontINDEX) GetPrivate(glyphID uint32) (*cffPrivateDICT, error) {
	i, ok := t.Index(glyphID)
	if !ok {
		return nil, fmt.Errorf("bad glyph ID %v", glyphID)
	}
	return t.private[i], nil
}

func (t *cffFontINDEX) GetLocalSubrs(glyphID uint32) (*cffINDEX, error) {
	i, ok := t.Index(glyphID)
	if !ok {
		return nil, fmt.Errorf("bad glyph ID %v", glyphID)
	}
	return t.localSubrs[i], nil
}

func parseFontINDEX(b []byte, fdArray, fdSelect, nGlyphs int, isCFF2 bool) (*cffFontINDEX, error) {
	if len(b) < fdArray {
		return nil, fmt.Errorf("bad Font INDEX offset")
	}

	r := NewBinaryReader(b)
	r.Seek(uint32(fdArray))
	fontINDEX, err := parseINDEX(r, false)
	if err != nil {
		return nil, fmt.Errorf("Font INDEX: %w", err)
	}

	fonts := &cffFontINDEX{}
	fonts.private = make([]*cffPrivateDICT, fontINDEX.Len())
	fonts.localSubrs = make([]*cffINDEX, fontINDEX.Len())
	for i := 0; i < fontINDEX.Len(); i++ {
		fontDICT, err := parseFontDICT(fontINDEX.Get(uint16(i)), isCFF2)
		if err != nil {
			return nil, fmt.Errorf("Font DICT: %w", err)
		}
		if len(b) < fontDICT.PrivateOffset || len(b)-fontDICT.PrivateOffset < fontDICT.PrivateLength {
			return nil, fmt.Errorf("Font DICT: bad Private DICT offset")
		}
		privateDICT, err := parsePrivateDICT(b[fontDICT.PrivateOffset:fontDICT.PrivateOffset+fontDICT.PrivateLength], isCFF2)
		if err != nil {
			return nil, fmt.Errorf("Private DICT: %w", err)
		}
		fonts.private[i] = privateDICT

		if privateDICT.Subrs != 0 {
			if len(b)-fontDICT.PrivateOffset < privateDICT.Subrs {
				return nil, fmt.Errorf("bad Local Subrs INDEX offset")
			}
			r.Seek(uint32(fontDICT.PrivateOffset + privateDICT.Subrs))
			fonts.localSubrs[i], err = parseINDEX(r, isCFF2)
			if err != nil {
				return nil, fmt.Errorf("Local Subrs INDEX: %w", err)
			}
		} else if isCFF2 {
			return nil, fmt.Errorf("Private DICT must have Local Subrs INDEX offset")
		}
	}

	r.Seek(uint32(fdSelect))
	format := r.ReadUint8()
	if format == 0 {
		fonts.fds = make([]uint8, nGlyphs)
		for i := 0; i < nGlyphs; i++ {
			fonts.fds[i] = r.ReadUint8()
		}
	} else if format == 3 {
		nRanges := r.ReadUint16()
		fonts.first = make([]uint32, nRanges+1)
		fonts.fd = make([]uint16, nRanges)
		for i := 0; i < int(nRanges); i++ {
			fonts.first[i] = uint32(r.ReadUint16())
			fonts.fd[i] = uint16(r.ReadUint8())
		}
		fonts.first[nRanges] = uint32(r.ReadUint16())
	} else if isCFF2 && format == 4 {
		nRanges := r.ReadUint32()
		fonts.first = make([]uint32, nRanges+1)
		fonts.fd = make([]uint16, nRanges)
		for i := 0; i < int(nRanges); i++ {
			fonts.first[i] = r.ReadUint32()
			fonts.fd[i] = r.ReadUint16()
		}
		fonts.first[nRanges] = r.ReadUint32()
	} else {
		return nil, fmt.Errorf("FDSelect: bad format")
	}
	return fonts, nil
}

// return added size when appending an offset to a dict, including the size of the offset number
// in the DICT
func cffDICTAppendedOffsetSize(offset int) int {
	nInt := 5
	if offset+1 <= 107 {
		nInt = 1
	} else if 108 <= offset+2 && offset+2 <= 1131 {
		nInt = 2
	} else if offset+3 <= 32767 {
		nInt = 3
	}

	_, nFloat := cffDICTFloat(float64(offset))
	if nInt < nFloat {
		return nInt
	}
	for {
		// length may increase as offset moves further
		oldNFloat := nFloat
		_, nFloat = cffDICTFloat(float64(offset + nFloat))
		if nFloat == oldNFloat {
			break
		}
	}
	return nFloat
}

func (cff *cffTable) Write() ([]byte, error) {
	if cff.version != 1 {
		return nil, fmt.Errorf("unsupported version: %d", cff.version)
	}

	if 1 < len(cff.fonts.private) || 1 < len(cff.fonts.localSubrs) {
		return nil, fmt.Errorf("must contain only one font")
	}

	w := NewBinaryWriter([]byte{})
	w.WriteUint8(1) // major version
	w.WriteUint8(0) // minor version
	w.WriteUint8(4) // hdrSize
	w.WriteUint8(1) // offSize, not used

	name := &cffINDEX{}
	name.Add([]byte(cff.name))
	nameINDEX, err := name.Write()
	if err != nil {
		return nil, fmt.Errorf("Name INDEX: %v", err)
	}
	w.WriteBytes(nameINDEX)

	strings := &cffINDEX{}
	topDICT, err := cff.top.Write(strings)
	if err != nil {
		return nil, fmt.Errorf("Top DICT: %v", err)
	}

	stringINDEX, err := strings.Write()
	if err != nil {
		return nil, fmt.Errorf("String INDEX: %v", err)
	}

	globalSubrsINDEX, err := cff.globalSubrs.Write()
	if err != nil {
		return nil, fmt.Errorf("Global Subrs INDEX: %v", err)
	}

	charStringsINDEX, err := cff.charStrings.Write()
	if err != nil {
		return nil, fmt.Errorf("CharStrings INDEX: %v", err)
	}

	var privateDICT []byte
	var localSubrsINDEX []byte
	localSubrsOffset := 0
	if len(cff.fonts.private) != 0 {
		privateDICT, err = cff.fonts.private[0].Write()
		if err != nil {
			return nil, fmt.Errorf("Private DICT: %v", err)
		}

		if len(cff.fonts.localSubrs) != 0 {
			localSubrsINDEX, err = cff.fonts.localSubrs[0].Write()
			if err != nil {
				return nil, fmt.Errorf("Local Subrs INDEX: %v", err)
			}

			// write offset to Private DICT
			localSubrsOffset = len(privateDICT) + 1                         // key
			localSubrsOffset += cffDICTAppendedOffsetSize(localSubrsOffset) // val
			wPrivate := &BinaryWriter{privateDICT}
			writeDICTEntry(wPrivate, 19, localSubrsOffset)
			privateDICT = wPrivate.Bytes()
		}
	}

	// the charStringsOffset and privateOffset values in Top DICT can each occupy a maximum of
	// 6 bytes (key + operator + uint32). The Top DICT INDEX takes 2 + 1 + 2*offSize + len(data)
	maxTopDICT := len(topDICT) + 1 + 5 // key and offset
	if privateDICT != nil {
		maxTopDICT += 1 + 5 + cffDICTNumberSize(len(privateDICT)) // key, offset, and length
	}
	maxTopDICTINDEXOffSize := cffINDEXOffSize(maxTopDICT)
	maxTopDICTINDEX := 3 + 2*maxTopDICTINDEXOffSize + maxTopDICT
	charStringsOffset := int(w.Len()) + maxTopDICTINDEX + len(stringINDEX) + len(globalSubrsINDEX)
	privateOffset := charStringsOffset + len(charStringsINDEX)
	if math.MaxUint32 < privateOffset {
		return nil, fmt.Errorf("offset too large")
	}

	// correct for maximum offset calculated above
	correct, prevCorrect := 0, -1
	for correct != prevCorrect {
		prevCorrect = correct
		correct = 5 - cffDICTAppendedOffsetSize(charStringsOffset) // number length in DICT
		if privateDICT != nil {
			correct += 5 - cffDICTAppendedOffsetSize(privateOffset) // number length in DICT
		}
		correct += maxTopDICTINDEXOffSize - cffINDEXOffSize(maxTopDICT-correct) // offSize in INDEX
	}
	charStringsOffset -= correct
	privateOffset -= correct

	// write offsets to Top DICT
	wTop := &BinaryWriter{topDICT}
	writeDICTEntry(wTop, 17, charStringsOffset)
	if privateDICT != nil {
		writeDICTEntry(wTop, 18, len(privateDICT), privateOffset)
	}
	topDICT = wTop.Bytes()

	// create Top DICT INDEX
	top := &cffINDEX{}
	top.Add(topDICT)
	topDICTINDEX, err := top.Write()
	if err != nil {
		return nil, fmt.Errorf("Top DICT INDEX: %v", err)
	}

	// write out all data
	w.WriteBytes(topDICTINDEX)
	w.WriteBytes(stringINDEX)
	w.WriteBytes(globalSubrsINDEX)
	w.WriteBytes(charStringsINDEX)
	w.WriteBytes(privateDICT)
	w.WriteBytes(localSubrsINDEX)
	return w.Bytes(), nil
}
