package prompt

import (
	"fmt"

	"github.com/cloudwego/eino/schema"
)

func AnalyzerParagraph(v AnalyzerParagraphVars) ([]*schema.Message, error) {
	return eng.messages(v,
		part{schema.System, "analyzer/paragraph_system.jinja"},
		part{schema.User, "analyzer/paragraph_user.jinja"})
}

func AnalyzerToolDescription() (string, error) {
	return eng.render("analyzer/tool_submit_half.jinja", nil)
}

func AnalyzerArticle(v AnalyzerArticleVars) ([]*schema.Message, error) {
	return eng.messages(v,
		part{schema.System, "analyzer/article_system.jinja"},
		part{schema.User, "analyzer/article_user.jinja"})
}

// AnalyzerRepair 渲染 JSON 解析失败后追加在对话末尾的用户消息。
func AnalyzerRepair(v AnalyzerRepairVars) ([]*schema.Message, error) {
	return eng.messages(v, part{schema.User, "analyzer/repair.jinja"})
}

func Scaffolder(v ScaffolderVars) ([]*schema.Message, error) {
	return eng.messages(v,
		part{schema.System, "scaffolder/system.jinja"},
		part{schema.User, "scaffolder/user.jinja"})
}

// 工具名，与 scaffolder 包注册的工具一一对应。
const (
	ToolReadArticle     = "read_article"
	ToolListCollection  = "list_collection"
	ToolReadArticleByID = "read_article_by_id"
	ToolSearchMaterials = "search_materials"
	ToolReadOutline     = "read_outline"
	ToolReadGuide       = "read_guide"
)

var toolNames = []string{
	ToolReadArticle, ToolListCollection, ToolReadArticleByID,
	ToolSearchMaterials, ToolReadOutline, ToolReadGuide,
}

var toolDescriptions = mustToolDescriptions()

func mustToolDescriptions() map[string]string {
	m := make(map[string]string, len(toolNames))
	for _, n := range toolNames {
		s, err := eng.render("scaffolder/tool_"+n+".jinja", nil)
		if err != nil {
			panic(fmt.Errorf("tool description %s: %w", n, err))
		}
		m[n] = s
	}
	return m
}

func ToolDescription(name string) string { return toolDescriptions[name] }

// Probe 是设置页“测试连接”发送的极短请求。
func Probe() ([]*schema.Message, error) {
	return eng.messages(nil, part{schema.User, "common/probe.jinja"})
}
