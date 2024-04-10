package font

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
)

var (
	KeepAllTables = []string{"all"}
	KeepMinTables = []string{"min"}
	KeepPDFTables = []string{"pdf"}
)

type SubsetOptions struct {
	Tables []string
}

// Subset trims an SFNT font to contain only the passed glyphIDs, thereby resulting in a significant size reduction. The glyphIDs will apear in the specified order in the file and their dependencies are added to the end.
func (sfnt *SFNT) Subset(glyphIDs []uint16, options SubsetOptions) *SFNT {
	if sfnt.IsCFF {
		if _, ok := sfnt.Tables["CFF2"]; ok {
			// TODO: support subsetting CFF2
			return sfnt
		}
	}

	// set up glyph mapping from original to subset
	glyphMap := make(map[uint16]uint16, len(glyphIDs))
	for subsetGlyphID, glyphID := range glyphIDs {
		glyphMap[glyphID] = uint16(subsetGlyphID)
	}

	if sfnt.IsTrueType {
		// add dependencies for composite glyphs add the end
		origLen := len(glyphIDs)
		for i := 0; i < origLen; i++ {
			if glyphIDs[i] == 0 {
				continue
			}
			deps, err := sfnt.Glyf.Dependencies(glyphIDs[i])
			if err != nil {
				panic(err)
			}
			for _, glyphID := range deps[1:] {
				if _, ok := glyphMap[glyphID]; !ok {
					subsetGlyphID := uint16(len(glyphIDs))
					glyphIDs = append(glyphIDs, glyphID)
					glyphMap[glyphID] = subsetGlyphID
				}
			}
		}
	}

	// specify tables to include
	var tags []string
	if len(options.Tables) == 1 && options.Tables[0] == "min" {
		tags = []string{"DSIG", "cmap", "head", "hhea", "hmtx", "maxp", "name", "OS/2", "post"}
		if sfnt.IsTrueType {
			tags = append(tags, "glyf", "loca")
		} else if sfnt.IsCFF {
			if _, ok := sfnt.Tables["CFF2"]; ok {
				tags = append(tags, "CFF2")
			} else {
				tags = append(tags, "CFF ")
			}
		}
	} else if len(options.Tables) == 1 && options.Tables[0] == "pdf" {
		if sfnt.IsTrueType {
			tags = append(tags, "DSIG", "glyf", "head", "hhea", "hmtx", "loca", "maxp")
			for _, tag := range []string{"cvt ", "fpgm", "prep"} {
				if _, ok := sfnt.Tables[tag]; ok {
					tags = append(tags, tag)
				}
			}
		} else if sfnt.IsCFF {
			// head and maxp tables are needed for almost all viewers
			// hhea and hmtx tables are needed for Evince (and possibly other viewers)
			// DSIG is recommended by Microsoft's FontValidator
			if _, ok := sfnt.Tables["CFF2"]; ok {
				tags = append(tags, "DSIG", "cmap", "CFF2", "head", "hhea", "hmtx", "maxp") // not strictly allowed
			} else {
				tags = append(tags, "DSIG", "cmap", "CFF ", "head", "hhea", "hmtx", "maxp")
			}
		}
	} else if len(options.Tables) == 1 && options.Tables[0] == "all" {
		for tag := range sfnt.Tables {
			tags = append(tags, tag)
		}
	} else {
		tags = options.Tables
	}
	sort.Strings(tags) // so that glyf is before loca

	// preliminary calculations
	indexToLocFormat := int16(1)                   // for head and loca
	glyfOffsets := make([]uint32, len(glyphIDs)+1) // for loca
	ulUnicodeRange := [4]uint32{0, 0, 0, 0}        // for OS/2
	numberOfHMetrics := uint16(len(glyphIDs))      // for hhea and hmtx
	if 1 < numberOfHMetrics {
		advance := sfnt.Hmtx.Advance(glyphIDs[numberOfHMetrics-1])
		for 1 < numberOfHMetrics {
			if sfnt.Hmtx.Advance(glyphIDs[numberOfHMetrics-2]) != advance {
				break
			}
			numberOfHMetrics--
		}
	}

	// copy to new SFNT
	sfntOld := sfnt
	sfnt = &SFNT{
		Version:    sfntOld.Version,
		Length:     12, // increased below
		IsCFF:      sfntOld.IsCFF,
		IsTrueType: sfntOld.IsTrueType,
		Tables:     map[string][]byte{},
		Loca:       &locaTable{}, // for glyf
	}

	// copy and rewrite tables
	for _, tag := range tags {
		if tag == "maxp" {
			sfnt.Maxp = &(*sfntOld.Maxp)
			sfnt.Maxp.NumGlyphs = uint16(len(glyphIDs))
			break
		}
	}
	for _, tag := range tags {
		table, ok := sfntOld.Tables[tag]
		if !ok {
			continue
		}

		switch tag {
		case "cmap":
			rs := make([]rune, 0, len(glyphIDs))
			runeMap := make(map[rune]uint16, len(glyphIDs)) // for OS/2
			for subsetGlyphID, glyphID := range glyphIDs {
				if r := sfntOld.Cmap.ToUnicode(glyphID); r != 0 {
					rs = append(rs, r)
					runeMap[r] = uint16(subsetGlyphID)
				}
			}

			w := NewBinaryWriter([]byte{})
			w.WriteUint16(0)  // version
			w.WriteUint16(1)  // numTables
			w.WriteUint16(0)  // platformID
			w.WriteUint16(4)  // encodingID
			w.WriteUint32(12) // subtableOffset

			// format 12
			start := w.Len()
			w.WriteUint16(12) // format
			w.WriteUint16(0)  // reserved
			w.WriteUint32(16) // length (updated later)
			w.WriteUint32(0)  // language
			w.WriteUint32(0)  // numGroups (set later)

			if 0 < len(rs) {
				sort.Slice(rs, func(i, j int) bool { return rs[i] < rs[j] })

				numGroups := uint32(1)
				startCharCode := uint32(rs[0])
				startGlyphID := uint32(runeMap[rs[0]])
				n := uint32(1)
				for i := 1; i < len(rs); i++ {
					r := rs[i]
					subsetGlyphID := runeMap[r]
					if r == rs[i-1] {
						continue
					} else if uint32(r) == startCharCode+n && uint32(subsetGlyphID) == startGlyphID+n {
						n++
					} else {
						w.WriteUint32(uint32(startCharCode))         // startCharCode
						w.WriteUint32(uint32(startCharCode + n - 1)) // endCharCode
						w.WriteUint32(uint32(startGlyphID))          // startGlyphID
						numGroups++
						startCharCode = uint32(r)
						startGlyphID = uint32(subsetGlyphID)
						n = 1
					}
				}
				w.WriteUint32(uint32(startCharCode))         // startCharCode
				w.WriteUint32(uint32(startCharCode + n - 1)) // endCharCode
				w.WriteUint32(uint32(startGlyphID))          // startGlyphID

				binary.BigEndian.PutUint32(w.buf[start+4:], w.Len()-start) // set length
				binary.BigEndian.PutUint32(w.buf[start+12:], numGroups)    // set numGroups
				ulUnicodeRange = os2UlUnicodeRange(rs)
			}
			sfnt.Tables[tag] = w.Bytes()

			if err := sfnt.parseCmap(); err != nil {
				panic("invalid cmap table: " + err.Error())
			}
		case "CFF ":
			cff := *sfntOld.CFF
			cff.charStrings = &cffINDEX{}
			for _, glyphID := range glyphIDs {
				charString := sfntOld.CFF.charStrings.Get(glyphID)
				if charString == nil {
					panic(fmt.Sprintf("bad glyphID: %v", glyphID))
				}
				cff.charStrings.Add(charString) // copies data
			}

			// trim globalSubrs and localSubrs INDEX
			if err := cff.rearrangeSubrs(); err != nil {
				panic("CFF table: " + err.Error())
			}

			b, err := cff.Write()
			if err != nil {
				panic("invalid CFF table: " + err.Error())
			}
			sfnt.Tables[tag] = b

			sfnt.CFF = &cff
		case "glyf":
			w := NewBinaryWriter([]byte{})
			for i, glyphID := range glyphIDs {
				if glyphID == 0 {
					// empty .notdef
					glyfOffsets[i+1] = w.Len()
					continue
				} else if sfntOld.Glyf.IsComposite(glyphID) {
					// composite glyphs, update glyphIDs and make sure not to write to b
					b := sfntOld.Glyf.Get(glyphID)
					start := w.Len()
					w.WriteBytes(b)

					offset := uint32(10)
					for {
						flags := binary.BigEndian.Uint16(b[offset:])
						subGlyphID := binary.BigEndian.Uint16(b[offset+2:])
						binary.BigEndian.PutUint16(w.buf[start+offset+2:], glyphMap[subGlyphID])

						length, more := glyfCompositeLength(flags)
						if !more {
							break
						}
						offset += length
					}
				} else {
					// simple glyph
					contour, err := sfntOld.Glyf.Contour(glyphID)
					if err != nil {
						// bad glyf data or bug in Contour, write original glyph data
						w.WriteBytes(sfntOld.Glyf.Get(glyphID))
					} else if 0 < len(contour.EndPoints) { // not empty
						// optimize glyph data
						numberOfContours := int16(len(contour.EndPoints))
						w.WriteInt16(numberOfContours)
						w.WriteInt16(contour.XMin)
						w.WriteInt16(contour.YMin)
						w.WriteInt16(contour.XMax)
						w.WriteInt16(contour.YMax)
						for _, endPoint := range contour.EndPoints {
							w.WriteUint16(endPoint)
						}
						w.WriteUint16(uint16(len(contour.Instructions)))
						w.WriteBytes(contour.Instructions)

						repeats := 0
						xs := NewBinaryWriter([]byte{})
						ys := NewBinaryWriter([]byte{})
						numPoints := int(contour.EndPoints[numberOfContours-1]) + 1
						for i := 0; i < numPoints; i++ {
							dx := contour.XCoordinates[i]
							dy := contour.YCoordinates[i]
							if 0 < i {
								dx -= contour.XCoordinates[i-1]
								dy -= contour.YCoordinates[i-1]
							}

							var flag byte
							if dx == 0 {
								flag |= 0x10 // X_IS_SAME_OR_POSITIVE_X_SHORT_VECTOR
							} else if -256 < dx && dx < 256 {
								flag |= 0x02 // X_SHORT_VECTOR
								if 0 < dx {
									flag |= 0x10 // X_IS_SAME_OR_POSITIVE_X_SHORT_VECTOR
									xs.WriteInt8(int8(dx))
								} else {
									xs.WriteInt8(int8(-dx))
								}
							} else {
								xs.WriteInt16(dx)
							}

							if dy == 0 {
								flag |= 0x20 // Y_IS_SAME_OR_POSITIVE_Y_SHORT_VECTOR
							} else if -256 < dy && dy < 256 {
								flag |= 0x04 // Y_SHORT_VECTOR
								if 0 < dy {
									flag |= 0x20 // Y_IS_SAME_OR_POSITIVE_Y_SHORT_VECTOR
									ys.WriteByte(byte(dy))
								} else {
									ys.WriteByte(byte(-dy))
								}
							} else {
								ys.WriteInt16(dy)
							}

							if contour.OnCurve[i] {
								flag |= 0x01
							}
							if contour.OverlapSimple[i] {
								flag |= 0x40
							}

							// handle flag repeats
							if 0 < i && repeats < 255 && flag == w.buf[len(w.buf)-1] {
								repeats++
							} else {
								if 1 < repeats {
									w.buf[len(w.buf)-1] |= 0x08 // REPEAT_FLAG
									w.WriteByte(byte(repeats))
									repeats = 0
								} else if repeats == 1 {
									w.WriteByte(w.buf[len(w.buf)-1])
									repeats = 0
								}
								w.WriteByte(flag)
							}
						}
						if 1 < repeats {
							w.buf[len(w.buf)-1] |= 0x08 // REPEAT_FLAG
							w.WriteByte(byte(repeats))
						} else if repeats == 1 {
							w.WriteByte(w.buf[len(w.buf)-1])
						}
						w.WriteBytes(xs.Bytes())
						w.WriteBytes(ys.Bytes())
					}
				}

				// padding to ensure glyph offsets are on even bytes for loca short format
				if w.Len()%2 == 1 {
					w.WriteByte(0)
				}
				glyfOffsets[i+1] = w.Len()
			}
			if w.Len() <= math.MaxUint16 {
				indexToLocFormat = 0 // short format
			}
			sfnt.Tables[tag] = w.Bytes()

			sfnt.Glyf = &glyfTable{
				data: w.Bytes(),
				loca: sfnt.Loca,
			}
		case "head":
			w := NewBinaryWriter(make([]byte, 0, len(sfntOld.Tables["head"])))
			w.WriteBytes(table[:50])
			w.WriteInt16(indexToLocFormat) // indexToLocFormat
			w.WriteBytes(table[52:])
			sfnt.Tables[tag] = w.Bytes()

			sfnt.Head = &(*sfntOld.Head)
			sfnt.Head.IndexToLocFormat = indexToLocFormat
		case "hhea":
			w := NewBinaryWriter(make([]byte, 0, len(sfntOld.Tables["hhea"])))
			w.WriteBytes(table[:34])
			w.WriteUint16(numberOfHMetrics) // numberOfHMetrics
			w.WriteBytes(table[36:])
			sfnt.Tables[tag] = w.Bytes()

			sfnt.Hhea = &(*sfntOld.Hhea)
			sfnt.Hhea.NumberOfHMetrics = numberOfHMetrics
		case "hmtx":
			sfnt.Hmtx = &hmtxTable{}
			sfnt.Hmtx.HMetrics = make([]hmtxLongHorMetric, numberOfHMetrics)
			sfnt.Hmtx.LeftSideBearings = make([]int16, len(glyphIDs)-int(numberOfHMetrics))

			n := 4*int(numberOfHMetrics) + 2*(len(glyphIDs)-int(numberOfHMetrics))
			w := NewBinaryWriter(make([]byte, 0, n))
			for subsetGlyphID, glyphID := range glyphIDs {
				lsb := sfntOld.Hmtx.LeftSideBearing(glyphID)
				if subsetGlyphID < int(numberOfHMetrics) {
					adv := sfntOld.Hmtx.Advance(glyphID)
					sfnt.Hmtx.HMetrics[subsetGlyphID].AdvanceWidth = adv
					sfnt.Hmtx.HMetrics[subsetGlyphID].LeftSideBearing = lsb
					w.WriteUint16(adv)
				} else {
					sfnt.Hmtx.LeftSideBearings[subsetGlyphID-int(numberOfHMetrics)] = lsb
				}
				w.WriteInt16(lsb)
			}
			sfnt.Tables[tag] = w.Bytes()
		case "kern":
			// handle kern table that could be removed
			kernSubtables := []kernFormat0{}
			for _, subtable := range sfntOld.Kern.Subtables {
				pairs := []kernPair{}
				for l, lOrig := range glyphIDs {
					if lOrig == 0 {
						continue
					}
					for r, rOrig := range glyphIDs {
						if rOrig == 0 {
							continue
						}
						if value := subtable.Get(lOrig, rOrig); value != 0 {
							pairs = append(pairs, kernPair{
								Key:   uint32(l)<<16 + uint32(r),
								Value: value,
							})
						}
					}
				}
				if 0 < len(pairs) {
					kernSubtables = append(kernSubtables, kernFormat0{
						Coverage: subtable.Coverage,
						Pairs:    pairs,
					})
				}
			}
			if len(kernSubtables) == 0 {
				continue
			}

			w := NewBinaryWriter([]byte{})
			w.WriteUint16(0)                          // version
			w.WriteUint16(uint16(len(kernSubtables))) // nTables
			for _, subtable := range kernSubtables {
				w.WriteUint16(0)                                     // version
				w.WriteUint16(6 + 8 + 6*uint16(len(subtable.Pairs))) // length
				w.WriteUint8(0)                                      // format
				w.WriteUint8(flagsToUint8(subtable.Coverage))        // coverage

				nPairs := uint16(len(subtable.Pairs))
				entrySelector := uint16(math.Log2(float64(nPairs)))
				searchRange := uint16(1 << entrySelector * 6)
				w.WriteUint16(nPairs)
				w.WriteUint16(searchRange)
				w.WriteUint16(entrySelector)
				w.WriteUint16(nPairs*6 - searchRange)
				for _, pair := range subtable.Pairs {
					w.WriteUint32(pair.Key)
					w.WriteInt16(pair.Value)
				}
			}
			sfnt.Tables[tag] = w.Bytes()

			if err := sfnt.parseKern(); err != nil {
				panic("invalid kern table: " + err.Error())
			}
		case "loca":
			var w *BinaryWriter
			if indexToLocFormat == 0 {
				// short format
				w = NewBinaryWriter(make([]byte, 2*len(glyfOffsets)))
				for _, offset := range glyfOffsets {
					w.WriteUint16(uint16(offset / 2))
				}
			} else {
				// long format
				w = NewBinaryWriter(make([]byte, 4*len(glyfOffsets)))
				for _, offset := range glyfOffsets {
					w.WriteUint32(offset)
				}
			}
			sfnt.Tables[tag] = w.Bytes()

			sfnt.Loca.Format = indexToLocFormat
			sfnt.Loca.data = w.Bytes()
		case "maxp":
			w := NewBinaryWriter(make([]byte, 0, len(sfntOld.Tables["maxp"])))
			w.WriteBytes(table[:4])
			w.WriteUint16(uint16(len(glyphIDs))) // numGlyphs
			w.WriteBytes(table[6:])
			sfnt.Tables[tag] = w.Bytes()
		case "name":
			w := NewBinaryWriter(make([]byte, 0, 6))
			w.WriteUint16(0) // version
			w.WriteUint16(0) // count
			w.WriteUint16(6) // storageOffset
			sfnt.Tables[tag] = w.Bytes()

			sfnt.Name = &nameTable{}
		case "OS/2":
			sfnt.OS2 = sfntOld.OS2
			sfnt.OS2.UlUnicodeRange1 = ulUnicodeRange[0]
			sfnt.OS2.UlUnicodeRange2 = ulUnicodeRange[1]
			sfnt.OS2.UlUnicodeRange3 = ulUnicodeRange[2]
			sfnt.OS2.UlUnicodeRange4 = ulUnicodeRange[3]

			w := NewBinaryWriter(make([]byte, 0, len(sfntOld.Tables["OS/2"])))
			w.WriteBytes(table[:42])
			w.WriteUint32(sfnt.OS2.UlUnicodeRange1)
			w.WriteUint32(sfnt.OS2.UlUnicodeRange2)
			w.WriteUint32(sfnt.OS2.UlUnicodeRange3)
			w.WriteUint32(sfnt.OS2.UlUnicodeRange4)
			w.WriteBytes(table[58:])
			sfnt.Tables[tag] = w.Bytes()
		case "post":
			w := NewBinaryWriter(make([]byte, 0, 32))
			w.WriteUint32(0x00030000) // version
			w.WriteBytes(table[4:32])
			sfnt.Tables[tag] = w.Bytes()

			sfnt.Post = &postTable{
				ItalicAngle:        sfntOld.Post.ItalicAngle,
				UnderlinePosition:  sfntOld.Post.UnderlinePosition,
				UnderlineThickness: sfntOld.Post.UnderlineThickness,
				IsFixedPitch:       sfntOld.Post.IsFixedPitch,
				MinMemType42:       sfntOld.Post.MinMemType42,
				MaxMemType42:       sfntOld.Post.MaxMemType42,
				MinMemType1:        sfntOld.Post.MinMemType1,
				MaxMemType1:        sfntOld.Post.MaxMemType1,
			}
		default:
			sfnt.Tables[tag] = table
		}

		sfnt.Length += uint32(16 + len(sfnt.Tables[tag]))        // 16 for the table record
		sfnt.Length += (4 - uint32(len(sfnt.Tables[tag]))&3) & 3 // padding
	}
	return sfnt
}

func (sfnt *SFNT) SetGlyphNames(names []string) error {
	table, ok := sfnt.Tables["post"]
	if !ok {
		return fmt.Errorf("post table doesn't exist")
	}

	sfnt.Post.NumGlyphs = sfnt.NumGlyphs()
	sfnt.Post.GlyphNameIndex = make([]uint16, sfnt.NumGlyphs())
	sfnt.Post.stringData = [][]byte{}
	sfnt.Post.nameMap = nil

	w := NewBinaryWriter(make([]byte, 0, 34+2*sfnt.NumGlyphs()))
	w.WriteUint32(0x00020000) // version
	w.WriteBytes(table[4:32])
	w.WriteUint16(sfnt.NumGlyphs()) // numGyphs

	lastIndex := uint16(258)
	stringData := NewBinaryWriter([]byte{})
	if int(sfnt.NumGlyphs()) < len(names) {
		names = names[:sfnt.NumGlyphs()]
	}
	for glyphID, name := range names {
		if 255 < len(name) {
			return fmt.Errorf("name too long for glyph ID %d", glyphID)
		}

		var index uint16
		for i, macintoshName := range macintoshGlyphNames {
			if name == macintoshName {
				index = uint16(i)
				break
			}
		}
		if index == 0 && name != ".notdef" {
			for i, prevName := range names[:glyphID] {
				if name == prevName {
					index = uint16(sfnt.Post.GlyphNameIndex[i])
					break
				}
			}

			if index == 0 {
				index = lastIndex
				stringData.WriteByte(byte(len(name)))
				stringData.WriteString(name)
				sfnt.Post.stringData = append(sfnt.Post.stringData, []byte(name))
				lastIndex++
			}
		}
		sfnt.Post.GlyphNameIndex[glyphID] = index
		w.WriteUint16(index)
	}
	for glyphID := uint16(len(names)); glyphID < sfnt.NumGlyphs(); glyphID++ {
		w.WriteUint16(0) // .notdef
	}
	w.Write(stringData.Bytes())

	sfnt.Tables["post"] = w.Bytes()
	return nil
}

func os2UlUnicodeRange(rs []rune) [4]uint32 {
	v := [4]uint32{0, 0, 0, 0}
	for _, r := range rs {
		if bit := os2UlUnicodeRangeBit(r); bit != -1 {
			i := int(bit / 32)
			v[3-i] |= 2 << (bit - i*32)
		}
		if 0x10000 <= r && r < 0x110000 {
			v[2] |= 2 << 25 // bit 57 (Non-Plane 0)
		}
	}
	return v
}

func os2UlUnicodeRangeBit(r rune) int {
	if r < 0x80 {
		return 0
	} else if r < 0x0100 {
		return 1
	} else if r < 0x0180 {
		return 2
	} else if r < 0x0250 {
		return 3
	} else if r < 0x02B0 {
		return 4
	} else if r < 0x0300 {
		return 5
	} else if r < 0x0370 {
		return 6
	} else if r < 0x0400 {
		return 7
	} else if r < 0x0500 {
		return 9
	} else if r < 0x0530 {
		return -1
	} else if r < 0x0590 {
		return 10
	} else if r < 0x0600 {
		return 11
	} else if r < 0x0700 {
		return 13
	} else if r < 0x0750 {
		return 71
	} else if r < 0x0780 {
		return -1
	} else if r < 0x07C0 {
		return 72
	} else if r < 0x0800 {
		return 14
	} else if r < 0x0900 {
		return -1
	} else if r < 0x0980 {
		return 15
	} else if r < 0x0A00 {
		return 16
	} else if r < 0x0A80 {
		return 17
	} else if r < 0x0B00 {
		return 18
	} else if r < 0x0B80 {
		return 19
	} else if r < 0x0C00 {
		return 20
	} else if r < 0x0C80 {
		return 21
	} else if r < 0x0D00 {
		return 22
	} else if r < 0x0D80 {
		return 23
	} else if r < 0x0E00 {
		return 73
	} else if r < 0x0E80 {
		return 24
	} else if r < 0x0F00 {
		return 25
	} else if r < 0x1000 {
		return 70
	} else if r < 0x10A0 {
		return 74
	} else if r < 0x1100 {
		return 26
	} else if r < 0x1200 {
		return 28
	} else if r < 0x1380 {
		return 75
	} else if r < 0x13A0 {
		return -1
	} else if r < 0x1400 {
		return 76
	} else if r < 0x1680 {
		return 77
	} else if r < 0x16A0 {
		return 78
	} else if r < 0x1700 {
		return 79
	} else if r < 0x1720 {
		return 84
	} else if r < 0x1780 {
		return -1
	} else if r < 0x1800 {
		return 80
	} else if r < 0x18B0 {
		return 81
	} else if r < 0x1900 {
		return -1
	} else if r < 0x1950 {
		return 93
	} else if r < 0x1980 {
		return 94
	} else if r < 0x19E0 {
		return 95
	} else if r < 0x1A00 {
		return -1
	} else if r < 0x1A20 {
		return 96
	} else if r < 0x1B00 {
		return -1
	} else if r < 0x1B80 {
		return 27
	} else if r < 0x1BC0 {
		return 112
	} else if r < 0x1C00 {
		return -1
	} else if r < 0x1C50 {
		return 113
	} else if r < 0x1C80 {
		return 114
	} else if r < 0x1E00 {
		return -1
	} else if r < 0x1F00 {
		return 29
	} else if r < 0x2000 {
		return 30
	} else if r < 0x2070 {
		return 31
	} else if r < 0x20A0 {
		return 32
	} else if r < 0x20D0 {
		return 33
	} else if r < 0x2100 {
		return 34
	} else if r < 0x2150 {
		return 35
	} else if r < 0x2190 {
		return 36
	} else if r < 0x2200 {
		return 37
	} else if r < 0x2300 {
		return 38
	} else if r < 0x2400 {
		return 39
	} else if r < 0x2440 {
		return 40
	} else if r < 0x2460 {
		return 41
	} else if r < 0x2500 {
		return 42
	} else if r < 0x2580 {
		return 43
	} else if r < 0x25A0 {
		return 44
	} else if r < 0x2600 {
		return 45
	} else if r < 0x2700 {
		return 46
	} else if r < 0x27C0 {
		return 47
	} else if r < 0x2800 {
		return -1
	} else if r < 0x2900 {
		return 82
	} else if r < 0x2C00 {
		return -1
	} else if r < 0x2C60 {
		return 97
	} else if r < 0x2C80 {
		return -1
	} else if r < 0x2D00 {
		return 8
	} else if r < 0x2D30 {
		return -1
	} else if r < 0x2D80 {
		return 98
	} else if r < 0x3000 {
		return -1
	} else if r < 0x3040 {
		return 48
	} else if r < 0x30A0 {
		return 49
	} else if r < 0x3100 {
		return 50
	} else if r < 0x3130 {
		return 51
	} else if r < 0x3190 {
		return 52
	} else if r < 0x3200 {
		return -1
	} else if r < 0x3300 {
		return 54
	} else if r < 0x3400 {
		return 55
	} else if r < 0x31C0 {
		return -1
	} else if r < 0x31F0 {
		return 61
	} else if r < 0x4DC0 {
		return -1
	} else if r < 0x4E00 {
		return 99
	} else if r < 0xA000 {
		return 59
	} else if r < 0xA490 {
		return 83
	} else if r < 0xA500 {
		return -1
	} else if r < 0xA640 {
		return 12
	} else if r < 0xA800 {
		return -1
	} else if r < 0xA830 {
		return 100
	} else if r < 0xA840 {
		return -1
	} else if r < 0xA880 {
		return 53
	} else if r < 0xA8E0 {
		return 115
	} else if r < 0xA900 {
		return -1
	} else if r < 0xA930 {
		return 116
	} else if r < 0xA960 {
		return 117
	} else if r < 0xAA00 {
		return -1
	} else if r < 0xAA60 {
		return 118
	} else if r < 0xAC00 {
		return -1
	} else if r < 0xD7AF {
		return 56
	} else if r < 0xE00 {
		return -1
	} else if r < 0xF900 {
		return 60
	} else if r < 0xFB00 {
		return -1
	} else if r < 0xFB50 {
		return 62
	} else if r < 0xFE00 {
		return 63
	} else if r < 0xFE10 {
		return 91
	} else if r < 0xFE20 {
		return 65
	} else if r < 0xFE30 {
		return 64
	} else if r < 0xFE50 {
		return -1
	} else if r < 0xFE70 {
		return 66
	} else if r < 0xFF00 {
		return 67
	} else if r < 0xFFF0 {
		return 68
	} else if r < 0x10000 {
		return 69
	} else if r < 0x10080 {
		return 101
	} else if r < 0x10140 {
		return -1
	} else if r < 0x10190 {
		return 102
	} else if r < 0x101D0 {
		return 119
	} else if r < 0x10200 {
		return 120
	} else if r < 0x102A0 {
		return -1
	} else if r < 0x102E0 {
		return 121
	} else if r < 0x10300 {
		return -1
	} else if r < 0x10330 {
		return 85
	} else if r < 0x10350 {
		return 86
	} else if r < 0x10380 {
		return -1
	} else if r < 0x103A0 {
		return 103
	} else if r < 0x103E0 {
		return 104
	} else if r < 0x10400 {
		return -1
	} else if r < 0x10450 {
		return 87
	} else if r < 0x10480 {
		return 105
	} else if r < 0x104B0 {
		return 106
	} else if r < 0x10800 {
		return -1
	} else if r < 0x10840 {
		return 107
	} else if r < 0x10A00 {
		return -1
	} else if r < 0x10A60 {
		return 108
	} else if r < 0x12000 {
		return -1
	} else if r < 0x12400 {
		return 110
	} else if r < 0x1D000 {
		return -1
	} else if r < 0x1D100 {
		return 88
	} else if r < 0x1D300 {
		return -1
	} else if r < 0x1D360 {
		return 109
	} else if r < 0x1D380 {
		return 111
	} else if r < 0x1D400 {
		return -1
	} else if r < 0x1D800 {
		return 89
	} else if r < 0x1F030 {
		return -1
	} else if r < 0x1F0A0 {
		return 122
	} else if r < 0xE0000 {
		return -1
	} else if r < 0xE0080 {
		return 92
	} else if r < 0xF0000 {
		return -1
	} else if r < 0xFFFFE {
		return 90
	}
	return -1
}
