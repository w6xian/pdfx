# 更新日志

## v1.1.0

发布日期：2026-10-20

### 新增

- **内存输出**：新增 `WithWriter(w io.Writer)` 选项，可把 PDF 直接写入任意 `io.Writer`
  （`*bytes.Buffer`、`http.ResponseWriter` 等），设置后不再落盘，`WithSavePath` 被忽略。
- **字节输出**：新增 `DrawHTMLBytes(html string) ([]byte, error)`，一次性返回 PDF 字节，
  无需临时文件，适合 HTTP 接口直接返回下载。

```go
// 方式一：拿到字节后自己控制响应
data, err := p.DrawHTMLBytes(htmlStr)
w.Header().Set("Content-Type", "application/pdf")
w.Header().Set("Content-Disposition", `attachment; filename="report.pdf"`)
w.Write(data)

// 方式二：流式写入 http.ResponseWriter
pdfx.NewPDFPaper(pdfx.WithWriter(w), pdfx.WithStyle(pdfx.A4)).DrawHTML(htmlStr)
```

- **HTTP 示例**：`examples/basic` 新增 `-http :8080` 模式，启动后访问
  `/download/stream`（`WithWriter` 直写响应）或 `/download/bytes`（`DrawHTMLBytes`）即可下载 PDF。

### 变更

- 内部渲染主流程 `_draw_pdf` 增加 writer 参数：writer 非 nil 时走 `pdf.Write(w)`，
  否则沿用原写文件逻辑（对外行为不变）。
- `examples/basic` 抽出公共的 `prepare()`（模板 + 字体准备），`Render` / `renderTo` / `renderBytes` 复用。

### 兼容性

- 完全向后兼容：`WithSavePath` + `DrawHTML` / `DrawFile` 的用法与输出保持不变，
  仅新增可选能力，无破坏性改动。
- 依赖与 Go 版本要求不变（`github.com/signintech/gopdf v0.38.1`、`golang.org/x/net v0.59.0`）。

### 验证

- `go build ./...`、`go vet ./...`、`go test ./...` 全部通过。
- 冒烟：分别用 `WithWriter` 与 `DrawHTMLBytes` 渲染同一份 HTML，输出以 `%PDF-1.7` 开头、字节数一致。

### 升级

```bash
go get github.com/w6xian/pdfx@v1.1.0
```
