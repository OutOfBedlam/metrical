package echarts

import "html/template"

var tmplFuncMap = template.FuncMap{}

var tmpl = template.Must(template.New("index").Funcs(tmplFuncMap).Parse(`
<script src="https://cdn.jsdelivr.net/npm/echarts@6.0.0/dist/echarts.min.js"></script>
`))
