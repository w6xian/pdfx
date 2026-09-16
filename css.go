package pdfx

import (
	"strings"

	"golang.org/x/net/html"
)

type simpleStyleSheet struct {
	Rules []cssRule
}

type cssRuleType int

const (
	ruleTag cssRuleType = iota
	ruleClass
)

type cssRule struct {
	typ      cssRuleType
	selector string
	decls    []cssDecl
	order    int
}

type cssDecl struct {
	key   string
	value string
	order int
}

type cssWinner struct {
	value  string
	weight int
	order  int
}

func parseSimpleStyleSheet(cssText string) simpleStyleSheet {
	css := stripCSSComments(cssText)
	out := simpleStyleSheet{}
	ruleOrder := 0

	for {
		i := strings.IndexByte(css, '{')
		if i < 0 {
			break
		}
		j := strings.IndexByte(css[i+1:], '}')
		if j < 0 {
			break
		}
		j = i + 1 + j

		selPart := strings.TrimSpace(css[:i])
		declPart := strings.TrimSpace(css[i+1 : j])
		css = css[j+1:]

		if selPart == "" || declPart == "" {
			continue
		}

		decls := parseDeclarationsOrdered(declPart)
		if len(decls) == 0 {
			continue
		}

		selectors := strings.Split(selPart, ",")
		for _, s := range selectors {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if strings.HasPrefix(s, ".") {
				name := strings.TrimSpace(strings.TrimPrefix(s, "."))
				if name == "" {
					continue
				}
				out.Rules = append(out.Rules, cssRule{typ: ruleClass, selector: name, decls: decls, order: ruleOrder})
				ruleOrder++
				continue
			}
			tag := strings.ToLower(strings.TrimSpace(s))
			if tag == "" {
				continue
			}
			if strings.ContainsAny(tag, " .#>:+[") {
				continue
			}
			out.Rules = append(out.Rules, cssRule{typ: ruleTag, selector: tag, decls: decls, order: ruleOrder})
			ruleOrder++
		}
	}

	return out
}

func stripCSSComments(s string) string {
	for {
		i := strings.Index(s, "/*")
		if i < 0 {
			break
		}
		j := strings.Index(s[i+2:], "*/")
		if j < 0 {
			s = s[:i]
			break
		}
		j = i + 2 + j
		s = s[:i] + s[j+2:]
	}
	return s
}

func parseDeclarationsOrdered(s string) []cssDecl {
	parts := strings.Split(s, ";")
	out := make([]cssDecl, 0, len(parts))
	order := 0
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		kv := strings.SplitN(p, ":", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(kv[0]))
		v := strings.TrimSpace(kv[1])
		if k == "" || v == "" {
			continue
		}
		out = append(out, cssDecl{key: k, value: v, order: order})
		order++
	}
	return out
}

func collectStyleText(pdfNode *html.Node) string {
	var b strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "style") {
			txt := extractText(n)
			if strings.TrimSpace(txt) != "" {
				b.WriteString(txt)
				b.WriteString("\n")
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(pdfNode)
	return b.String()
}

func applyStyleSheetToTree(root *html.Node, sheet simpleStyleSheet) {
	var walk func(n *html.Node, inherited map[string]string)
	walk = func(n *html.Node, inherited map[string]string) {
		nextInherited := inherited
		if n.Type == html.ElementNode && !strings.EqualFold(n.Data, "style") {
			computed := applyComputedStyle(n, inherited, sheet)
			nextInherited = pickInheritedStyle(computed)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, nextInherited)
		}
	}
	walk(root, nil)
}

func applyComputedStyle(n *html.Node, inherited map[string]string, sheet simpleStyleSheet) map[string]string {
	winners := map[string]cssWinner{}
	if inherited != nil {
		for k, v := range inherited {
			winners[k] = cssWinner{value: v, weight: 0, order: 0}
		}
	}

	tagName := strings.ToLower(n.Data)
	for _, r := range sheet.Rules {
		if r.typ != ruleTag || r.selector != tagName {
			continue
		}
		for _, d := range r.decls {
			ord := r.order*1000 + d.order
			applyWinner(winners, d.key, d.value, 100, ord)
		}
	}

	classes := classListOrdered(n)
	for ci, cls := range classes {
		for _, r := range sheet.Rules {
			if r.typ != ruleClass || r.selector != cls {
				continue
			}
			for _, d := range r.decls {
				ord := ci*1000000 + r.order*1000 + d.order
				applyWinner(winners, d.key, d.value, 1000, ord)
			}
		}
	}

	inlineDecls := parseDeclarationsOrdered(getAttr(n, "style"))
	for _, d := range inlineDecls {
		applyWinner(winners, d.key, d.value, 10000, d.order)
	}

	computed := map[string]string{}
	for k, w := range winners {
		if strings.TrimSpace(w.value) == "" {
			continue
		}
		computed[k] = w.value
	}
	if len(computed) > 0 {
		setAttr(n, "style", serializeDeclarations(computed))
	}
	return computed
}

func applyWinner(winners map[string]cssWinner, key, value string, weight, order int) {
	if key == "" {
		return
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	cur, ok := winners[key]
	if !ok || weight > cur.weight || (weight == cur.weight && order >= cur.order) {
		winners[key] = cssWinner{value: value, weight: weight, order: order}
	}
}

func classListOrdered(n *html.Node) []string {
	v := strings.TrimSpace(getAttr(n, "class"))
	if v == "" {
		return nil
	}
	parts := strings.Fields(v)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func pickInheritedStyle(computed map[string]string) map[string]string {
	if len(computed) == 0 {
		return nil
	}
	out := map[string]string{}
	for _, k := range []string{"font-family", "font-size", "font-weight", "line-height", "color", "white-space"} {
		if v := strings.TrimSpace(computed[k]); v != "" {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
