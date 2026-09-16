package pdfx

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math/bits"
	"os"
	"path/filepath"
	"strings"

	"github.com/signintech/gopdf"
	"golang.org/x/net/html"
)

func addFont(pdf *gopdf.GoPdf, family, fontPath string, ttcIndex int) error {
	ext := strings.ToLower(filepath.Ext(fontPath))
	if ext != ".ttc" {
		return pdf.AddTTFFont(family, fontPath)
	}

	ttfData, err := ttcToTTF(fontPath, ttcIndex)
	if err != nil {
		return err
	}
	return pdf.AddTTFFontByReader(family, bytes.NewReader(ttfData))
}

func ttcToTTF(ttcPath string, index int) ([]byte, error) {
	ttc, err := os.ReadFile(ttcPath)
	if err != nil {
		return nil, err
	}
	if len(ttc) < 12 {
		return nil, errors.New("ttc too small")
	}
	if string(ttc[0:4]) != "ttcf" {
		return nil, errors.New("not a ttc file")
	}

	numFonts := int(binary.BigEndian.Uint32(ttc[8:12]))
	if numFonts <= 0 {
		return nil, errors.New("ttc has no fonts")
	}
	if index < 0 || index >= numFonts {
		return nil, errors.New("ttc font index out of range")
	}

	offBase := 12
	if len(ttc) < offBase+numFonts*4 {
		return nil, errors.New("invalid ttc header")
	}
	fontOffset := int(binary.BigEndian.Uint32(ttc[offBase+index*4 : offBase+index*4+4]))
	if fontOffset < 0 || fontOffset+12 > len(ttc) {
		return nil, errors.New("invalid font offset")
	}

	scalerType := binary.BigEndian.Uint32(ttc[fontOffset : fontOffset+4])
	numTables := int(binary.BigEndian.Uint16(ttc[fontOffset+4 : fontOffset+6]))
	if numTables <= 0 {
		return nil, errors.New("invalid numTables")
	}

	tableDirStart := fontOffset + 12
	tableDirSize := numTables * 16
	if tableDirStart+tableDirSize > len(ttc) {
		return nil, errors.New("invalid table directory")
	}

	type tableRec struct {
		tag      [4]byte
		checksum uint32
		offset   uint32
		length   uint32
	}
	recs := make([]tableRec, 0, numTables)
	for i := 0; i < numTables; i++ {
		p := tableDirStart + i*16
		var tag [4]byte
		copy(tag[:], ttc[p:p+4])
		recs = append(recs, tableRec{
			tag:      tag,
			checksum: binary.BigEndian.Uint32(ttc[p+4 : p+8]),
			offset:   binary.BigEndian.Uint32(ttc[p+8 : p+12]),
			length:   binary.BigEndian.Uint32(ttc[p+12 : p+16]),
		})
	}

	headerSize := 12 + numTables*16
	out := make([]byte, headerSize)
	binary.BigEndian.PutUint32(out[0:4], scalerType)
	binary.BigEndian.PutUint16(out[4:6], uint16(numTables))

	maxPow2 := 1 << (bits.Len(uint(numTables)) - 1)
	searchRange := maxPow2 * 16
	entrySelector := bits.Len(uint(maxPow2)) - 1
	rangeShift := numTables*16 - searchRange
	binary.BigEndian.PutUint16(out[6:8], uint16(searchRange))
	binary.BigEndian.PutUint16(out[8:10], uint16(entrySelector))
	binary.BigEndian.PutUint16(out[10:12], uint16(rangeShift))

	writePos := headerSize
	newRecs := make([]tableRec, len(recs))
	copy(newRecs, recs)

	for i := range newRecs {
		srcOff := int(newRecs[i].offset)
		srcLen := int(newRecs[i].length)
		if srcOff < 0 || srcLen < 0 || srcOff+srcLen > len(ttc) {
			return nil, errors.New("invalid table offset/length")
		}

		alignedPos := align4(writePos)
		if alignedPos > writePos {
			out = append(out, make([]byte, alignedPos-writePos)...)
			writePos = alignedPos
		}

		newRecs[i].offset = uint32(writePos)
		out = append(out, ttc[srcOff:srcOff+srcLen]...)
		writePos += srcLen

		alignedEnd := align4(writePos)
		if alignedEnd > writePos {
			out = append(out, make([]byte, alignedEnd-writePos)...)
			writePos = alignedEnd
		}
	}

	for i := 0; i < numTables; i++ {
		p := 12 + i*16
		copy(out[p:p+4], newRecs[i].tag[:])
		binary.BigEndian.PutUint32(out[p+4:p+8], newRecs[i].checksum)
		binary.BigEndian.PutUint32(out[p+8:p+12], newRecs[i].offset)
		binary.BigEndian.PutUint32(out[p+12:p+16], newRecs[i].length)
	}

	return out, nil
}

func align4(n int) int {
	return (n + 3) &^ 3
}

type fontState struct {
	family string
	style  string
	size   float64
}

func setFontIfNeeded(pdf *gopdf.GoPdf, family, style string, size float64, fs *fontState) error {
	_, err := setFontIfNeededWithFakeBold(pdf, family, style, size, fs)
	return err
}

func setFontIfNeededWithFakeBold(pdf *gopdf.GoPdf, family, style string, size float64, fs *fontState) (bool, error) {
	if fs != nil && fs.family == family && fs.style == style && fs.size == size {
		return false, nil
	}
	if err := pdf.SetFont(family, style, size); err != nil {
		if err == gopdf.ErrMissingFontFamily && strings.Contains(strings.ToUpper(style), "B") {
			if err2 := pdf.SetFont(family, "", size); err2 != nil {
				return false, err2
			}
			if fs != nil {
				fs.family = family
				fs.style = ""
				fs.size = size
			}
			return true, nil
		}
		return false, err
	}
	if fs != nil {
		fs.family = family
		fs.style = style
		fs.size = size
	}
	return false, nil
}

func setPdfFontFromNode(pdf *gopdf.GoPdf, n *html.Node) error {
	if pdf == nil || n == nil {
		return nil
	}
	family := strings.TrimSpace(parseStyleValue(n, "font-family"))
	if family != "" {
		family = parseFirstFontFamily(family)
	}
	size := 0.0
	if v := strings.TrimSpace(parseStyleValue(n, "font-size")); v != "" {
		size = parseFontSizeToPoints(v)
	}
	if family == "" && size <= 0 {
		return nil
	}
	if family == "" {
		if f, ok := getGoPdfCurrentFontFamily(pdf); ok {
			family = f
		}
	}
	if size <= 0 {
		if s, ok := getGoPdfCurrentFontSizePoints(pdf); ok {
			size = s
		}
	}
	if family == "" || size <= 0 {
		return nil
	}
	style := ""
	if w := strings.TrimSpace(parseStyleValue(n, "font-weight")); w != "" && isBoldFontWeight(w) {
		style = "B"
	}
	if err := pdf.SetFont(family, style, size); err != nil {
		if err == gopdf.ErrMissingFontFamily && strings.Contains(strings.ToUpper(style), "B") {
			if err2 := pdf.SetFont(family, "", size); err2 != nil {
				return err2
			}
			return nil
		}
		return err
	}
	return nil
}

func parseFirstFontFamily(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	parts := strings.Split(v, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, `"'`)
		if p == "" {
			continue
		}
		return p
	}
	return ""
}
