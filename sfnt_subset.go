package font

import (
	"encoding/binary"
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
		// TODO: support subsetting CFF
		return sfnt
	}

	// set up glyph mapping from original to subset
	glyphMap := make(map[uint16]uint16, len(glyphIDs))
	for subsetGlyphID, glyphID := range glyphIDs {
		glyphMap[glyphID] = uint16(subsetGlyphID)
	}

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

	// specify tables to include
	var tags []string
	if len(options.Tables) == 1 && options.Tables[0] == "min" {
		tags = []string{"cmap", "head", "hhea", "hmtx", "maxp", "name", "OS/2", "post"}
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
			tags = append(tags, "glyf", "head", "hhea", "hmtx", "loca", "maxp")
			for _, tag := range []string{"cvt ", "fpgm", "prep"} {
				if _, ok := sfnt.Tables[tag]; ok {
					tags = append(tags, tag)
				}
			}
		} else if sfnt.IsCFF {
			if _, ok := sfnt.Tables["CFF2"]; ok {
				tags = append(tags, "CFF2", "cmap") // not strictly allowed
			} else {
				tags = append(tags, "CFF ", "cmap")
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
		table := sfntOld.Tables[tag]
		switch tag {
		case "cmap":
			rs := make([]rune, 0, len(glyphIDs))
			runeMap := make(map[rune]uint16, len(glyphIDs))
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
			}
			sfnt.Tables[tag] = w.Bytes()

			if err := sfnt.parseCmap(); err != nil {
				panic("invalid cmap table: " + err.Error())
			}
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
			sfnt.Tables[tag] = table

			sfnt.OS2 = sfntOld.OS2
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
