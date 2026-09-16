// Package pdfx 把一段受限的 HTML（含自定义标签）渲染成 PDF，基于 gopdf 实现。
//
// 典型用法：
//
//	p := pdfx.NewPDFPaper(
//		pdfx.WithStyle(pdfx.A4),
//		pdfx.WithPadding("5mm"),
//		pdfx.WithSavePath("out.pdf"),
//		pdfx.WithFonts(pdfx.NewFont("songti", "simsun.ttc")),
//	)
//	if err := p.DrawHTML(htmlStr); err != nil {
//		return err
//	}
//
// 渲染流程：
//
//	DrawHTML → parseHtmlToPdfNode（取出 <pdf> 根节点）
//	  → 标签别名 / 变量替换（template.go）
//	  → DrawPDFFromString（按节点类型分发：row / table / img / line / space）
//	  → 写盘
//
// 代码组织：
//
//	paper.go            门面：PDFPaper、Option、渲染主流程
//	page.go             纸张尺寸（PageStyle）、Paper、Padding
//	template.go         标签别名 Alias、变量 Var、v-if 表达式求值
//	css.go              <style> 简易样式表解析与继承
//	render_document.go  文档级渲染调度
//	render_row.go       row/col 布局与绘制
//	render_table.go     table 模型与绘制（含边框合并）
//	render_line.go      分隔线
//	render_image.go     图片（本地/网络/DataURI）
//	parse_row.go        row 标签与样式解析
//	parse_table.go      table 标签与单元格属性解析
//	layout.go           布局结构与列宽/盒子计算
//	style.go            颜色、高度、行高等样式解析
//	border.go           边框解析与生效宽度计算
//	text.go             文本测量、截断、换行
//	font.go             字体注册与 TTC 处理
//	node.go             HTML 节点工具
//	geom.go             几何工具（线段合并、端点延长）
package pdfx
