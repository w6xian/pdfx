package pdfx

import (
	"sort"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func findFirstElementFragment(fragment string, tag string) (*html.Node, error) {
	ctx := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(fragment), ctx)
	if err != nil {
		return nil, err
	}
	var walk func(n *html.Node) *html.Node
	walk = func(n *html.Node) *html.Node {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, tag) {
			return n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if found := walk(c); found != nil {
				return found
			}
		}
		return nil
	}
	for _, n := range nodes {
		if found := walk(n); found != nil {
			return found, nil
		}
	}
	return nil, nil
}

func onlyElementChild(n *html.Node, tag string) *html.Node {
	if n == nil || n.Type != html.ElementNode {
		return nil
	}
	var found *html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			if strings.TrimSpace(c.Data) != "" {
				return nil
			}
			continue
		}
		if c.Type != html.ElementNode {
			continue
		}
		if found != nil {
			return nil
		}
		if !strings.EqualFold(c.Data, tag) {
			return nil
		}
		found = c
	}
	return found
}

func normalizeSelfClosingTag(s, tag string) string {
	if strings.IndexByte(s, '<') < 0 {
		return s
	}
	if tag == "" {
		return s
	}

	isTerm := func(b byte) bool {
		switch b {
		case ' ', '\t', '\r', '\n', '>', '/':
			return true
		default:
			return false
		}
	}

	var out strings.Builder
	out.Grow(len(s) + 16)
	last := 0
	for i := 0; i < len(s); i++ {
		if s[i] != '<' {
			continue
		}
		if s[i+1] == '/' {
			continue
		}
		if i+1+len(tag) >= len(s) {
			continue
		}
		if !strings.EqualFold(s[i+1:i+1+len(tag)], tag) {
			continue
		}
		if i+1+len(tag) < len(s) && !isTerm(s[i+1+len(tag)]) {
			continue
		}
		j := strings.IndexByte(s[i+1:], '>')
		if j < 0 {
			break
		}
		j = i + 1 + j
		k := j - 1
		for k > i && (s[k] == ' ' || s[k] == '\t' || s[k] == '\r' || s[k] == '\n') {
			k--
		}
		if k <= i || s[k] != '/' {
			continue
		}

		out.WriteString(s[last:i])
		inner := strings.TrimRight(s[i+1:k], " \t\r\n")
		out.WriteByte('<')
		out.WriteString(inner)
		out.WriteString("></")
		out.WriteString(tag)
		out.WriteByte('>')
		last = j + 1
		i = j
	}
	out.WriteString(s[last:])
	return out.String()
}

func serializeDeclarations(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		v := strings.TrimSpace(m[k])
		if v == "" {
			continue
		}
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(v)
		b.WriteString("; ")
	}
	return strings.TrimSpace(b.String())
}

func findFirstElementInNodes(nodes []*html.Node, tag string) *html.Node {
	var walk func(n *html.Node) *html.Node
	walk = func(n *html.Node) *html.Node {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, tag) {
			return n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if found := walk(c); found != nil {
				return found
			}
		}
		return nil
	}
	for _, n := range nodes {
		if found := walk(n); found != nil {
			return found
		}
	}
	return nil
}
