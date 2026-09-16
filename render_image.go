package pdfx

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/signintech/gopdf"
	"golang.org/x/net/html"
)

import _ "image/gif"
import _ "image/jpeg"
import _ "image/png"

func DrawImageFromNode(pdf *gopdf.GoPdf, n *html.Node) error {
	if pdf == nil {
		return errors.New("img: nil GoPdf")
	}
	if n == nil || n.Type != html.ElementNode || !strings.EqualFold(n.Data, "img") {
		return nil
	}
	src := strings.TrimSpace(getAttr(n, "src"))
	if src == "" {
		return nil
	}

	left, _, right, _ := pdf.Margins()
	pageW, ok := getGoPdfPageWidthInUnits(pdf)
	contentW := 0.0
	if ok && pageW > 0 {
		contentW = pageW - left - right
	}
	if contentW <= 0 {
		contentW = 100
	}

	wStr := strings.TrimSpace(getAttr(n, "width"))
	if wStr == "" {
		wStr = strings.TrimSpace(parseStyleValue(n, "width"))
	}
	hStr := strings.TrimSpace(getAttr(n, "height"))
	if hStr == "" {
		hStr = strings.TrimSpace(parseStyleValue(n, "height"))
	}
	drawW := parseImgLengthToMM(wStr, contentW)
	drawH := parseImgLengthToMM(hStr, contentW)

	data, err := readImageBytes(src)
	if err != nil || len(data) == 0 {
		return err
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return err
	}

	imgWpx, imgHpx := cfg.Width, cfg.Height
	if imgWpx > 0 && imgHpx > 0 {
		if drawW > 0 && drawH <= 0 {
			drawH = drawW * float64(imgHpx) / float64(imgWpx)
		} else if drawH > 0 && drawW <= 0 {
			drawW = drawH * float64(imgWpx) / float64(imgHpx)
		} else if drawW <= 0 && drawH <= 0 {
			drawW = parseCSSLengthToMM(strconv.Itoa(imgWpx) + "px")
			drawH = parseCSSLengthToMM(strconv.Itoa(imgHpx) + "px")
		}
	}
	if drawW <= 0 || drawH <= 0 {
		return nil
	}

	holder, err := gopdf.ImageHolderByBytes(data)
	if err != nil {
		return err
	}

	// 绝对定位：x / left / right / y / top / bottom，原点为页面左上角（支持 mm、px、pt、%）
	pageH, _ := getGoPdfPageHeightInUnits(pdf)
	absX, hasAbsX := parseImgPosition(n, "x", "left", pageW)
	fromRight, hasFromRight := parseImgPosition(n, "right", "", pageW)
	absY, hasAbsY := parseImgPosition(n, "y", "top", pageH)
	fromBottom, hasFromBottom := parseImgPosition(n, "bottom", "", pageH)
	absolute := hasAbsX || hasFromRight || hasAbsY || hasFromBottom

	var x, y float64
	if absolute {
		switch {
		case hasAbsX:
			x = absX
		case hasFromRight:
			x = pageW - fromRight - drawW
		default:
			x = left
		}
		switch {
		case hasAbsY:
			y = absY
		case hasFromBottom:
			y = pageH - fromBottom - drawH
		default:
			y = pdf.GetY()
		}
	} else {
		// 流式：接在当前 Y 之后，按 align 水平排布
		y = ensurePageSpace(pdf, drawH)
		x = left
		align := strings.ToLower(strings.TrimSpace(getAttr(n, "align")))
		if align == "" {
			align = strings.ToLower(strings.TrimSpace(parseStyleValue(n, "text-align")))
		}
		marginStr := strings.ToLower(strings.TrimSpace(parseStyleValue(n, "margin")))
		if align == "" && strings.Contains(marginStr, "auto") {
			align = "center"
		}
		if contentW > drawW {
			switch align {
			case "center":
				x = left + (contentW-drawW)/2.0
			case "right":
				x = left + (contentW - drawW)
			}
		}
	}

	opts := gopdf.ImageOptions{X: x, Y: y, Rect: &gopdf.Rect{W: drawW, H: drawH}}
	if a, ok := parseImgRotate(n); ok {
		opts.DegreeAngle = a
	}
	if op, ok := parseImgOpacity(n); ok {
		// 混合模式要传 gopdf 的常量值（"/Normal"）：传 "Normal" 会被判为
		// unknown，之前错误被吞掉，opacity 一直没生效。
		tr, err := gopdf.NewTransparency(op, string(gopdf.NormalBlendMode))
		if err != nil {
			return err
		}
		opts.Transparency = &tr
	}
	if err := pdf.ImageByHolderWithOptions(holder, opts); err != nil {
		return err
	}
	if !absolute {
		pdf.SetY(y + drawH)
	}
	return nil
}

// parseImgPosition 读取图片定位值（x/left/right/y/top/bottom）
func parseImgPosition(n *html.Node, key, altKey string, pageSize float64) (float64, bool) {
	v := strings.TrimSpace(getAttr(n, key))
	if v == "" && altKey != "" {
		v = strings.TrimSpace(getAttr(n, altKey))
	}
	if v == "" {
		v = strings.TrimSpace(parseStyleValue(n, key))
		if v == "" && altKey != "" {
			v = strings.TrimSpace(parseStyleValue(n, altKey))
		}
	}
	if v == "" {
		return 0, false
	}
	v = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(strings.ToLower(v)), "!important"))
	if strings.HasSuffix(v, "%") {
		num, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(v, "%")), 64)
		if err != nil || pageSize <= 0 {
			return 0, false
		}
		return pageSize * num / 100.0, true
	}
	if isPureNumber(v) {
		return parseCSSLengthToMM(v + "px"), true
	}
	w := parseCSSLengthToMM(v)
	if w <= 0 {
		return 0, false
	}
	return w, true
}

// parseImgRotate 读取旋转角度：rotate="-18" 或 style="transform: rotate(-18deg)"
func parseImgRotate(n *html.Node) (float64, bool) {
	if v := strings.TrimSpace(getAttr(n, "rotate")); v != "" {
		v = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(v), "deg"))
		if a, err := strconv.ParseFloat(v, 64); err == nil {
			return a, true
		}
	}
	if s := strings.TrimSpace(parseStyleValue(n, "transform")); s != "" {
		low := strings.ToLower(s)
		if i := strings.Index(low, "rotate("); i >= 0 {
			rest := s[i+len("rotate("):]
			if j := strings.Index(rest, ")"); j >= 0 {
				num := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(rest[:j]), "deg"))
				if a, err := strconv.ParseFloat(num, 64); err == nil {
					return a, true
				}
			}
		}
	}
	return 0, false
}

// parseImgOpacity 读取透明度：opacity="0.8"
func parseImgOpacity(n *html.Node) (float64, bool) {
	v := strings.TrimSpace(getAttr(n, "opacity"))
	if v == "" {
		v = strings.TrimSpace(parseStyleValue(n, "opacity"))
	}
	if v == "" {
		return 0, false
	}
	v = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "!important"))
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, false
	}
	if f < 0 {
		f = 0
	}
	if f >= 1 {
		return 0, false
	}
	return f, true
}

func parseImgLengthToMM(v string, contentW float64) float64 {
	v = strings.TrimSpace(strings.ToLower(v))
	v = strings.TrimSpace(strings.TrimSuffix(v, "!important"))
	if v == "" || v == "auto" {
		return 0
	}
	if strings.HasSuffix(v, "%") {
		if contentW <= 0 {
			return 0
		}
		num, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(v, "%")), 64)
		if err != nil || num <= 0 {
			return 0
		}
		return contentW * num / 100.0
	}
	if isPureNumber(v) {
		return parseCSSLengthToMM(v + "px")
	}
	return parseCSSLengthToMM(v)
}

func readImageBytes(src string) ([]byte, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, errors.New("img: empty src")
	}
	if strings.HasPrefix(strings.ToLower(src), "data:") {
		return readDataImage(src)
	}
	if strings.HasPrefix(strings.ToLower(src), "http://") || strings.HasPrefix(strings.ToLower(src), "https://") {
		client := &http.Client{Timeout: 12 * time.Second}
		resp, err := client.Get(src)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, errors.New("img: http status " + resp.Status)
		}
		return io.ReadAll(resp.Body)
	}
	if strings.HasPrefix(strings.ToLower(src), "file://") {
		u, err := url.Parse(src)
		if err == nil {
			p := u.Path
			if u.Host != "" && p == "" {
				p = u.Host
			}
			p = filepath.FromSlash(p)
			if len(p) >= 3 && (p[0] == '\\' || p[0] == '/') && p[2] == ':' {
				c := p[1]
				if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
					p = p[1:]
				}
			}
			if p != "" {
				return os.ReadFile(p)
			}
		}
	}
	if abs, err := filepath.Abs(src); err == nil {
		if b, err2 := os.ReadFile(abs); err2 == nil {
			return b, nil
		}
	}
	return os.ReadFile(src)
}

func readDataImage(src string) ([]byte, error) {
	i := strings.Index(strings.ToLower(src), "base64,")
	if i < 0 {
		return nil, errors.New("img: data url missing base64")
	}
	raw := src[i+7:]
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("img: empty data")
	}
	return base64.StdEncoding.DecodeString(raw)
}
