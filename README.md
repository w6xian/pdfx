# image-pdf

把受限 HTML（含自定义标签）渲染成 PDF 的 Go 库与命令行示例。

## 目录结构

```
image-pdf/
├─ examples/basic/          # 命令行示例：渲染出厂检验报告（main 包）
│  ├─ main.go         # 入口、字体查找、输出
│  ├─ data.go         # 输入数据结构与模板视图
│  └─ report.tmpl     # HTML 模板
├─ doc.go          # 包文档：用法、渲染流程、文件职责
├─ paper.go        # 门面：PDFPaper + Option + 渲染主流程
├─ page.go         # 纸张尺寸 PageStyle / Paper / Padding
├─ template.go     # 标签别名 Alias、变量 Var、v-if 表达式
├─ css.go          # <style> 样式表解析与继承
├─ render_*.go     # 各节点类型的渲染实现
├─ parse_*.go      # 标签 / 属性解析
├─ layout.go       # 布局结构与列宽、盒子计算
├─ style.go        # 颜色、高度、行高解析
├─ border.go       # 边框解析与生效宽度
├─ text.go         # 文本测量、截断、换行
├─ font.go         # 字体注册与 TTC 处理
├─ node.go         # HTML 节点工具
└─ geom.go         # 几何工具（线段合并、端点延长）
└─ go.mod             # module pdfx
```


## 使用

```go
import "igithub.com/w6xian/pdfx"

p := pdfx.NewPDFPaper(
    pdfx.WithStyle(pdfx.A4),
    pdfx.WithPadding("5mm"),
    pdfx.WithSavePath("out.pdf"),
    pdfx.WithFonts(
        pdfx.NewFont("songti", `C:\Windows\Fonts\simsun.ttc`),
        pdfx.NewFont("heiti", `C:\Windows\Fonts\simhei.ttf`),
    ),
)
if err := p.DrawHTML(htmlStr); err != nil {
    return err
}
```

模板能力：

```go
p.Var("showSeal", true)                     // 供 v-if 使用
p.Alias("cell", "col", `align="center"`)    // 自定义标签别名
p.DrawFile("report.html")                   // 渲染 HTML 文件
```

## 命令

```powershell
make build   # 编译
make test    # 单元测试
make vet     # 静态检查
make fmt     # 格式化
make run     # 生成 report-x.pdf 示例
```

或直接用 go：`go run ./examples/basic/ -o report.pdf`。

## 约定

- 长度统一为 mm 单位（`Ratio` 定义于 `page.go`），字号为 pt（1pt = 25.4/72 mm）。
- 表格边框在绘制前会做共线合并（`geom.go`），避免同一条线重复描边导致变粗。
- 新增节点类型：在 `render_*.go` 增加绘制分支，并在 `doc.go` 的文件职责说明中补充。

## 注意事项

### 表格边框线宽

`<table border="1">` 里的 `1` 是 HTML 惯例，按 **1px** 解析，即 0.2646mm ≈ **0.75pt**。这个宽度在屏幕上会被抗锯齿摊成一段灰边，看着像「线没画实」。想要实心的细线，显式写单位：

```html
<table border="1pt" ...>     <!-- 1pt  = 0.3528mm -->
<table border="0.4mm" ...>   <!-- 0.4mm = 1.13pt  -->
```

规则见 `parseHTMLBorderWidth`：纯数字按 px，带单位（`pt` / `mm` / `cm` / `in`）按单位。
`border="1"` 和 `border="1mm"` 差着 3.8 倍，别混写。

### 表格边框颜色

在 `<table>` 上写，四种写法等价：

```html
<table border="1pt #333333" ...>                   <!-- 简写：宽度在前、颜色在后 -->
<table border="1pt" border-color="#333333" ...>
<table border="1pt" style="border-color:#333333;"> <!-- 内联 style -->
<table style="border: 1pt solid #333333;">         <!-- solid 会被忽略 -->
```

- 颜色支持 `#333` / `#333333` / `rgb(51,51,51)` / `black` / `white`（见 `parseColorSpec`），不写就是黑色。
- **带透明度也可以**：8 位 `#RRGGBBAA`、4 位 `#RGBA`、`rgba(51,51,51,0.88)` 三种写法都能解析出 alpha，描边时通过 gopdf 的 ExtGState 生效，画完立刻复位成不透明。注意 alpha 只对表格边框生效，文字/单元格背景会忽略它。
- 只认 3 / 4 / 6 / 8 位十六进制。位数不对（比如 5 位、7 位）会**整条解析失败并静默回退黑色**，不报错也不警告 —— 颜色没变化时先数一下位数。
- **必须写在 `<table>` 上**。表格的所有网格线共用 `Table.BorderColor`（`render_table.go` 只调一次 `SetStrokeColor`），写在 `<td>` / `<tr>` 上不生效。
- 也可以写在 `<style>` 的 `table { ... }` 规则里 —— 计算样式会被 `applyComputedStyle` 回写进元素的 `style` 属性，而 `parseStyleValue` 读的正是它。

改这块时注意一个坑：**`parseBorderValue` 在没写颜色时会补一个黑色**（`border.go` 末尾）。所以简写里的颜色不能直接用它的返回值，否则 `border="1pt" border-color="#333333"` 会被顶回黑色。`parseHTMLBorderColor` 为此单独走了 `colorInBorderShorthand`，逐字段试 `parseColorSpec`，只认真正出现的颜色词，`border="1pt"` / `border="none"` 都返回零值。边界用例锁在 `border_color_test.go`。

### 拐角封口

gopdf 的 `Line()` 每次输出一条独立路径（`m x1 y1 l x2 y2 S`），线端是 butt cap（PDF 默认 `0 j`），横竖两条线各自成路径、没有 line join。所以表格拐角处那个 `w × w` 的方块会缺一角：横线只盖住端点一侧，竖线只盖住另一侧，剩下 1/4 谁都没画，放大后就是「角上缺一个像素」。

`geom.go` 的 `extendLineEnds` 在绘制前把线段两端各延长 **w/2** 补齐。延长量正好落在垂直笔画的外沿，既补上缺口，也不会在 T 形接头处露出多余线头（gopdf 自带 tableLayout 用的是同一个办法）。自己画边框时记得走这个函数，别直接丢坐标给 `Line()`。

### 表格列宽（auto 布局）

列宽由 `render_table.go` 的 `ensureColWidthsFromHTML` 算出，几条容易踩的规则：

- **最小内容宽度只统计 `colspan=1` 的单元格**（文本实测宽度 + 左右 padding）。跨列单元格的内容**不会**分摊到任何单列上 —— 所以 `<td colspan="2">很长很长的一行字</td>` 不会把那两列撑开。
- **单元格上的 `width` 在跨列时是「这几列合计不小于这个值」**（CSS 语义），不是「每列这么宽」。缺口按各列最小内容宽度的**比例**分摊，不是平均摊 —— 平均摊会把内容多的列挤爆。跨度小的先算，内容已经够宽时不做调整。
- **`width:100%` 下，自动列按最小内容宽度的比例瓜分全部剩余空间**。因此当某个跨列是唯一的自动消费者时，它拿到的就恒等于剩余宽度，写在它上面的 `width` 只影响列间比例、不会改变总量；若该值小于内容所需宽度，则完全不起作用（最小值条件已满足）。

排查列宽不符预期时，先算一下 `表格宽 − 各显式列宽 = 剩余`，再比对跨列单元格内容的实测宽度，通常就能定位。
