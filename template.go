package pdfx

import (
	"reflect"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func parseAttrSpec(spec string) (key string, val string, ok bool) {
	s := strings.TrimSpace(spec)
	if s == "" {
		return "", "", false
	}
	kv := strings.SplitN(s, "=", 2)
	if len(kv) != 2 {
		return "", "", false
	}
	key = strings.TrimSpace(kv[0])
	val = strings.TrimSpace(kv[1])
	if key == "" {
		return "", "", false
	}
	if len(val) >= 2 {
		if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
			val = val[1 : len(val)-1]
		}
	}
	return key, val, true
}

func applyAliases(root *html.Node, aliases map[string]tagAlias) {
	if root == nil || len(aliases) == 0 {
		return
	}

	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n == nil {
			return
		}
		if n.Type == html.ElementNode {
			tag := strings.ToLower(strings.TrimSpace(n.Data))
			if a, ok := aliases[tag]; ok && a.baseTag != "" {
				n.Data = a.baseTag
				n.DataAtom = atom.Lookup([]byte(a.baseTag))
				mergeAttrs(n, a.attrs)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
}

func mergeAttrs(n *html.Node, add []html.Attribute) {
	if n == nil || len(add) == 0 {
		return
	}
	idx := make(map[string]int, len(n.Attr))
	for i, a := range n.Attr {
		k := strings.ToLower(strings.TrimSpace(a.Key))
		if k == "" {
			continue
		}
		idx[k] = i
	}

	for _, a := range add {
		k := strings.TrimSpace(a.Key)
		if k == "" {
			continue
		}
		kl := strings.ToLower(k)
		if kl == "style" {
			if i, ok := idx[kl]; ok {
				base := strings.TrimSpace(a.Val)
				existing := strings.TrimSpace(n.Attr[i].Val)
				if base == "" {
					continue
				}
				base = strings.TrimSuffix(base, ";")
				if existing == "" {
					n.Attr[i].Val = base
				} else {
					n.Attr[i].Val = base + ";" + existing
				}
				continue
			}
		}
		if _, ok := idx[kl]; ok {
			continue
		}
		n.Attr = append(n.Attr, html.Attribute{Key: k, Val: a.Val})
		idx[kl] = len(n.Attr) - 1
	}
}

func applyVIf(root *html.Node, vars map[string]any) {
	if root == nil || len(vars) == 0 {
		return
	}

	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n == nil {
			return
		}
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling
			if c.Type == html.ElementNode {
				expr := strings.TrimSpace(getAttr(c, "v-if"))
				if expr != "" {
					if !evalBoolExpr(expr, vars) {
						removeNode(c)
						c = next
						continue
					}
					removeAttr(c, "v-if")
				}
			}
			walk(c)
			c = next
		}
	}
	walk(root)
}

func removeNode(n *html.Node) {
	if n == nil || n.Parent == nil {
		return
	}
	p := n.Parent
	if n.PrevSibling != nil {
		n.PrevSibling.NextSibling = n.NextSibling
	} else {
		p.FirstChild = n.NextSibling
	}
	if n.NextSibling != nil {
		n.NextSibling.PrevSibling = n.PrevSibling
	} else {
		p.LastChild = n.PrevSibling
	}
	n.Parent = nil
	n.PrevSibling = nil
	n.NextSibling = nil
}

func removeAttr(n *html.Node, key string) {
	if n == nil || key == "" {
		return
	}
	out := n.Attr[:0]
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			continue
		}
		out = append(out, a)
	}
	n.Attr = out
}

func evalBoolExpr(expr string, vars map[string]any) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return false
	}
	expr = strings.TrimSpace(strings.TrimSuffix(expr, "!important"))

	partsOr := splitByOp(expr, "||")
	for _, partOr := range partsOr {
		partOr = strings.TrimSpace(partOr)
		if partOr == "" {
			continue
		}
		ok := true
		partsAnd := splitByOp(partOr, "&&")
		for _, partAnd := range partsAnd {
			partAnd = strings.TrimSpace(partAnd)
			if partAnd == "" {
				ok = false
				break
			}
			if !evalBoolTerm(partAnd, vars) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func splitByOp(expr string, op string) []string {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return []string{}
	}
	if op == "" {
		return []string{expr}
	}
	var out []string
	start := 0
	for i := 0; i+len(op) <= len(expr); i++ {
		if expr[i:i+len(op)] == op {
			out = append(out, expr[start:i])
			i += len(op) - 1
			start = i + 1
		}
	}
	out = append(out, expr[start:])
	return out
}

func evalBoolTerm(term string, vars map[string]any) bool {
	term = strings.TrimSpace(term)
	if term == "" {
		return false
	}

	neg := false
	for strings.HasPrefix(term, "!") {
		neg = !neg
		term = strings.TrimSpace(strings.TrimPrefix(term, "!"))
	}

	v := evalBoolAtom(term, vars)
	if neg {
		return !v
	}
	return v
}

func evalBoolAtom(atomExpr string, vars map[string]any) bool {
	atomExpr = strings.TrimSpace(atomExpr)
	if atomExpr == "" {
		return false
	}
	low := strings.ToLower(atomExpr)
	switch low {
	case "true":
		return true
	case "false":
		return false
	}
	if n, err := strconv.ParseFloat(atomExpr, 64); err == nil {
		return n != 0
	}
	val, ok := resolveVarPath(atomExpr, vars)
	if !ok {
		return false
	}
	return truthy(val)
}

func resolveVarPath(path string, vars map[string]any) (any, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false
	}
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return nil, false
	}
	rootName := strings.TrimSpace(parts[0])
	root, ok := vars[rootName]
	if !ok {
		return nil, false
	}
	cur := root
	for i := 1; i < len(parts); i++ {
		key := strings.TrimSpace(parts[i])
		if key == "" {
			return nil, false
		}
		next, ok := reflectGet(cur, key)
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func reflectGet(v any, key string) (any, bool) {
	if v == nil {
		return nil, false
	}
	if m, ok := v.(map[string]any); ok {
		val, ok2 := m[key]
		return val, ok2
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil, false
		}
		rv = rv.Elem()
	}

	switch rv.Kind() {
	case reflect.Struct:
		f := rv.FieldByName(key)
		if f.IsValid() {
			return f.Interface(), true
		}
		rt := rv.Type()
		for i := 0; i < rt.NumField(); i++ {
			sf := rt.Field(i)
			if strings.EqualFold(sf.Name, key) {
				ff := rv.Field(i)
				if ff.IsValid() {
					return ff.Interface(), true
				}
			}
		}
		return nil, false
	case reflect.Map:
		if rv.Type().Key().Kind() == reflect.String {
			mv := rv.MapIndex(reflect.ValueOf(key))
			if mv.IsValid() {
				return mv.Interface(), true
			}
		}
		return nil, false
	default:
		return nil, false
	}
}

func truthy(v any) bool {
	if v == nil {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		if s == "" {
			return false
		}
		if s == "false" || s == "0" {
			return false
		}
		return true
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return false
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return rv.Uint() != 0
	case reflect.Float32, reflect.Float64:
		return rv.Float() != 0
	case reflect.Bool:
		return rv.Bool()
	default:
		return !rv.IsZero()
	}
}
