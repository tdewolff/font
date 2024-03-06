package font

import (
	"fmt"
	"math"
)

type cmapFormat0 struct {
	GlyphIdArray [256]uint8

	unicodeMap map[uint16]rune
}

func (subtable *cmapFormat0) Get(r rune) (uint16, bool) {
	if r < 0 || 256 <= r {
		return 0, false
	}
	return uint16(subtable.GlyphIdArray[r]), true
}

func (subtable *cmapFormat0) ToUnicode(glyphID uint16) (rune, bool) {
	if 256 <= glyphID {
		return 0, false
	} else if subtable.unicodeMap == nil {
		subtable.unicodeMap = make(map[uint16]rune, 256)
		for r, id := range subtable.GlyphIdArray {
			subtable.unicodeMap[uint16(id)] = rune(r)
		}
	}
	r, ok := subtable.unicodeMap[glyphID]
	return r, ok
}

type cmapFormat4 struct {
	StartCode     []uint16
	EndCode       []uint16
	IdDelta       []int16
	IdRangeOffset []uint16
	GlyphIdArray  []uint16

	unicodeMap map[uint16]rune
}

func (subtable *cmapFormat4) Get(r rune) (uint16, bool) {
	if r < 0 || 65536 <= r {
		return 0, false
	}
	n := len(subtable.StartCode)
	for i := 0; i < n; i++ {
		if subtable.StartCode[i] <= uint16(r) && uint16(r) <= subtable.EndCode[i] {
			if subtable.IdRangeOffset[i] == 0 {
				// is modulo 65536 with the idDelta cast and addition overflow
				return uint16(subtable.IdDelta[i]) + uint16(r), true
			}
			// idRangeOffset/2  ->  offset value to index of words
			// r-startCode  ->  difference of rune with startCode
			// -(n-1)  ->  subtract offset from the current idRangeOffset item
			index := int(subtable.IdRangeOffset[i]/2) + int(uint16(r)-subtable.StartCode[i]) - (n - i)
			return subtable.GlyphIdArray[index], true // index is always valid
		}
	}
	return 0, false
}

func (subtable *cmapFormat4) ToUnicode(glyphID uint16) (rune, bool) {
	if subtable.unicodeMap == nil {
		subtable.unicodeMap = map[uint16]rune{}
		n := len(subtable.StartCode)
		for i := 0; i < n; i++ {
			for r := rune(subtable.StartCode[i]); r <= rune(subtable.EndCode[i]); r++ {
				var id uint16
				if subtable.IdRangeOffset[i] == 0 {
					// is modulo 65536 with the idDelta cast and addition overflow
					id = uint16(subtable.IdDelta[i]) + uint16(r)
				} else {
					// idRangeOffset/2  ->  offset value to index of words
					// r-startCode  ->  difference of rune with startCode
					// -(n-1)  ->  subtract offset from the current idRangeOffset item
					index := int(subtable.IdRangeOffset[i]/2) + int(uint16(r)-subtable.StartCode[i]) - (n - i)
					id = subtable.GlyphIdArray[index]
				}
				if _, ok := subtable.unicodeMap[id]; !ok {
					subtable.unicodeMap[id] = r
				}
			}
		}
	}
	r, ok := subtable.unicodeMap[glyphID]
	return r, ok
}

type cmapFormat6 struct {
	FirstCode    uint16
	GlyphIdArray []uint16
}

func (subtable *cmapFormat6) Get(r rune) (uint16, bool) {
	if r < int32(subtable.FirstCode) || uint32(len(subtable.GlyphIdArray)) <= uint32(r)-uint32(subtable.FirstCode) {
		return 0, false
	}
	return subtable.GlyphIdArray[uint32(r)-uint32(subtable.FirstCode)], true
}

func (subtable *cmapFormat6) ToUnicode(glyphID uint16) (rune, bool) {
	for i, id := range subtable.GlyphIdArray {
		if id == glyphID {
			return rune(subtable.FirstCode) + rune(i), true
		}
	}
	return 0, false
}

type cmapFormat12 struct {
	StartCharCode []uint32
	EndCharCode   []uint32
	StartGlyphID  []uint32

	unicodeMap map[uint16]rune
}

func (subtable *cmapFormat12) Get(r rune) (uint16, bool) {
	if r < 0 {
		return 0, false
	}
	for i := 0; i < len(subtable.StartCharCode); i++ {
		if subtable.StartCharCode[i] <= uint32(r) && uint32(r) <= subtable.EndCharCode[i] {
			return uint16((uint32(r) - subtable.StartCharCode[i]) + subtable.StartGlyphID[i]), true
		}
	}
	return 0, false
}

func (subtable *cmapFormat12) ToUnicode(glyphID uint16) (rune, bool) {
	if subtable.unicodeMap == nil {
		subtable.unicodeMap = map[uint16]rune{}
		for i := 0; i < len(subtable.StartCharCode); i++ {
			for r := subtable.StartCharCode[i]; r <= subtable.EndCharCode[i]; r++ {
				id := uint16((r - subtable.StartCharCode[i]) + subtable.StartGlyphID[i])
				if _, ok := subtable.unicodeMap[id]; !ok {
					subtable.unicodeMap[id] = rune(r)
				}
			}
		}
	}
	r, ok := subtable.unicodeMap[glyphID]
	return r, ok
}

type cmapEncodingRecord struct {
	PlatformID uint16
	EncodingID uint16
	Format     uint16
	Subtable   uint16
}

type cmapSubtable interface {
	Get(rune) (uint16, bool)
	ToUnicode(uint16) (rune, bool)
}

type cmapTable struct {
	EncodingRecords []cmapEncodingRecord
	Subtables       []cmapSubtable
}

func (cmap *cmapTable) Get(r rune) uint16 {
	for _, subtable := range cmap.Subtables {
		if glyphID, ok := subtable.Get(r); ok {
			return glyphID
		}
	}
	return 0
}

func (cmap *cmapTable) ToUnicode(glyphID uint16) rune {
	for _, subtable := range cmap.Subtables {
		if r, ok := subtable.ToUnicode(glyphID); ok {
			return r
		}
	}
	return 0
}

func (sfnt *SFNT) parseCmap() error {
	if sfnt.Maxp == nil {
		return fmt.Errorf("cmap: missing maxp table")
	}

	b, ok := sfnt.Tables["cmap"]
	if !ok {
		return fmt.Errorf("cmap: missing table")
	} else if len(b) < 4 {
		return fmt.Errorf("cmap: bad table")
	}

	sfnt.Cmap = &cmapTable{}
	r := NewBinaryReader(b)
	if r.ReadUint16() != 0 {
		return fmt.Errorf("cmap: bad version")
	}
	numTables := r.ReadUint16()
	if uint32(len(b)) < 4+8*uint32(numTables) {
		return fmt.Errorf("cmap: bad table")
	}

	// find and extract subtables and make sure they don't overlap each other
	offsets, lengths := []uint32{0}, []uint32{4 + 8*uint32(numTables)}
	for j := 0; j < int(numTables); j++ {
		platformID := r.ReadUint16()
		encodingID := r.ReadUint16()
		subtableID := -1

		offset := r.ReadUint32()
		if uint32(len(b))-8 < offset { // to extract the subtable format and length
			return fmt.Errorf("cmap: bad subtable %d", j)
		}
		for i := 0; i < len(offsets); i++ {
			if offsets[i] < offset && offset < lengths[i] {
				return fmt.Errorf("cmap: bad subtable %d", j)
			}
		}

		// extract subtable length
		rs := NewBinaryReader(b[offset:])
		format := rs.ReadUint16()
		var length uint32
		if format == 0 || format == 2 || format == 4 || format == 6 {
			length = uint32(rs.ReadUint16())
		} else if format == 8 || format == 10 || format == 12 || format == 13 {
			_ = rs.ReadUint16() // reserved
			length = rs.ReadUint32()
		} else if format == 14 {
			length = rs.ReadUint32()
		} else {
			return fmt.Errorf("cmap: bad format %d for subtable %d", format, j)
		}
		if length < 8 || math.MaxUint32-offset < length {
			return fmt.Errorf("cmap: bad subtable %d", j)
		}
		for i := 0; i < len(offsets); i++ {
			if offset == offsets[i] && length == lengths[i] {
				subtableID = int(i)
				break
			} else if offset <= offsets[i] && offsets[i] < offset+length {
				return fmt.Errorf("cmap: bad subtable %d", j)
			}
		}
		rs.buf = rs.buf[:length:length]

		if subtableID == -1 {
			subtableID = len(sfnt.Cmap.Subtables)
			offsets = append(offsets, offset)
			lengths = append(lengths, length)

			switch format {
			case 0:
				if rs.Len() != 258 {
					return fmt.Errorf("cmap: bad subtable %d", j)
				}
				_ = rs.ReadUint16() // languageID

				subtable := &cmapFormat0{}
				copy(subtable.GlyphIdArray[:], rs.ReadBytes(256))
				for _, glyphID := range subtable.GlyphIdArray {
					if sfnt.Maxp.NumGlyphs <= uint16(glyphID) {
						return fmt.Errorf("cmap: bad glyphID in subtable %d", j)
					}
				}
				sfnt.Cmap.Subtables = append(sfnt.Cmap.Subtables, subtable)
			case 4:
				if rs.Len() < 10 {
					return fmt.Errorf("cmap: bad subtable %d", j)
				}
				_ = rs.ReadUint16() // languageID

				segCount := rs.ReadUint16()
				if segCount%2 != 0 || segCount == 0 {
					return fmt.Errorf("cmap: bad segCount in subtable %d", j)
				}
				segCount /= 2
				if MaxCmapSegments < segCount {
					return fmt.Errorf("cmap: too many segments in subtable %d", j)
				}
				_ = rs.ReadUint16() // searchRange
				_ = rs.ReadUint16() // entrySelector
				_ = rs.ReadUint16() // rangeShift

				subtable := &cmapFormat4{}
				if rs.Len() < 2+8*uint32(segCount) {
					return fmt.Errorf("cmap: bad subtable %d", j)
				}
				subtable.EndCode = make([]uint16, segCount)
				for i := 0; i < int(segCount); i++ {
					endCode := rs.ReadUint16()
					if 0 < i && endCode <= subtable.EndCode[i-1] {
						return fmt.Errorf("cmap: bad endCode in subtable %d", j)
					}
					subtable.EndCode[i] = endCode
				}
				_ = rs.ReadUint16() // reservedPad
				subtable.StartCode = make([]uint16, segCount)
				for i := 0; i < int(segCount); i++ {
					startCode := rs.ReadUint16()
					if subtable.EndCode[i] < startCode || 0 < i && startCode <= subtable.EndCode[i-1] {
						return fmt.Errorf("cmap: bad startCode in subtable %d", j)
					}
					subtable.StartCode[i] = startCode
				}
				if subtable.StartCode[segCount-1] != 0xFFFF || subtable.EndCode[segCount-1] != 0xFFFF {
					return fmt.Errorf("cmap: bad last startCode or endCode in subtable %d", j)
				}

				subtable.IdDelta = make([]int16, segCount)
				for i := 0; i < int(segCount-1); i++ {
					subtable.IdDelta[i] = rs.ReadInt16()
				}
				_ = rs.ReadUint16() // last value may be invalid
				subtable.IdDelta[segCount-1] = 1

				glyphIdArrayLength := rs.Len() - 2*uint32(segCount)
				if glyphIdArrayLength%2 != 0 {
					return fmt.Errorf("cmap: bad subtable %d", j)
				}
				glyphIdArrayLength /= 2

				subtable.IdRangeOffset = make([]uint16, segCount)
				for i := 0; i < int(segCount-1); i++ {
					idRangeOffset := rs.ReadUint16()
					if idRangeOffset%2 != 0 {
						return fmt.Errorf("cmap: bad idRangeOffset in subtable %d", j)
					} else if idRangeOffset != 0 {
						index := int(idRangeOffset/2) + int(subtable.EndCode[i]-subtable.StartCode[i]) - (int(segCount) - i)
						if index < 0 || glyphIdArrayLength <= uint32(index) {
							return fmt.Errorf("cmap: bad idRangeOffset in subtable %d", j)
						}
					}
					subtable.IdRangeOffset[i] = idRangeOffset
				}
				_ = rs.ReadUint16() // last value may be invalid
				subtable.IdRangeOffset[segCount-1] = 0

				subtable.GlyphIdArray = make([]uint16, glyphIdArrayLength)
				for i := 0; i < int(glyphIdArrayLength); i++ {
					glyphID := rs.ReadUint16()
					if sfnt.Maxp.NumGlyphs <= glyphID {
						return fmt.Errorf("cmap: bad glyphID in subtable %d", j)
					}
					subtable.GlyphIdArray[i] = glyphID
				}
				sfnt.Cmap.Subtables = append(sfnt.Cmap.Subtables, subtable)
			case 6:
				if rs.Len() < 6 {
					return fmt.Errorf("cmap: bad subtable %d", j)
				}
				_ = rs.ReadUint16() // language

				subtable := &cmapFormat6{}
				subtable.FirstCode = rs.ReadUint16()
				entryCount := rs.ReadUint16()
				if rs.Len() < 2*uint32(entryCount) {
					return fmt.Errorf("cmap: bad subtable %d", j)
				}
				subtable.GlyphIdArray = make([]uint16, entryCount)
				for i := 0; i < int(entryCount); i++ {
					subtable.GlyphIdArray[i] = rs.ReadUint16()
				}
				sfnt.Cmap.Subtables = append(sfnt.Cmap.Subtables, subtable)
			case 12:
				if rs.Len() < 8 {
					return fmt.Errorf("cmap: bad subtable %d", j)
				}
				_ = rs.ReadUint32() // language
				numGroups := rs.ReadUint32()
				if MaxCmapSegments < numGroups {
					return fmt.Errorf("cmap: too many segments in subtable %d", j)
				} else if rs.Len() < 12*numGroups {
					return fmt.Errorf("cmap: bad subtable %d", j)
				}

				subtable := &cmapFormat12{}
				subtable.StartCharCode = make([]uint32, numGroups)
				subtable.EndCharCode = make([]uint32, numGroups)
				subtable.StartGlyphID = make([]uint32, numGroups)
				for i := 0; i < int(numGroups); i++ {
					startCharCode := rs.ReadUint32()
					endCharCode := rs.ReadUint32()
					startGlyphID := rs.ReadUint32()
					if endCharCode < startCharCode || 0 < i && startCharCode <= subtable.EndCharCode[i-1] {
						return fmt.Errorf("cmap: bad character code range in subtable %d", j)
					} else if uint32(sfnt.Maxp.NumGlyphs) <= endCharCode-startCharCode || uint32(sfnt.Maxp.NumGlyphs)-(endCharCode-startCharCode) <= startGlyphID {
						return fmt.Errorf("cmap: bad glyphID in subtable %d", j)
					}
					subtable.StartCharCode[i] = startCharCode
					subtable.EndCharCode[i] = endCharCode
					subtable.StartGlyphID[i] = startGlyphID
				}
				sfnt.Cmap.Subtables = append(sfnt.Cmap.Subtables, subtable)
			}
		}
		sfnt.Cmap.EncodingRecords = append(sfnt.Cmap.EncodingRecords, cmapEncodingRecord{
			PlatformID: platformID,
			EncodingID: encodingID,
			Format:     format,
			Subtable:   uint16(subtableID),
		})
	}
	return nil
}
