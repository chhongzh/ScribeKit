package scaffolder

import "strings"

const rationaleOpen = "<rationale>"

// deltaFilter 在流式输出里拦住 <rationale> 标签及其后的内容，只放行正文。
// 标签可能被拆在两个增量之间，所以结尾疑似标签前缀的部分先扣住。
type deltaFilter struct {
	pending string
	inTag   bool
	emit    func(string)
}

func (f *deltaFilter) write(chunk string) {
	if f.inTag || chunk == "" {
		return
	}
	f.pending += chunk
	if i := strings.Index(f.pending, rationaleOpen); i >= 0 {
		f.send(f.pending[:i])
		f.pending, f.inTag = "", true
		return
	}
	hold := 0
	for k := min(len(rationaleOpen)-1, len(f.pending)); k > 0; k-- {
		if strings.HasSuffix(f.pending, rationaleOpen[:k]) {
			hold = k
			break
		}
	}
	f.send(f.pending[:len(f.pending)-hold])
	f.pending = f.pending[len(f.pending)-hold:]
}

func (f *deltaFilter) flush() {
	if !f.inTag {
		f.send(f.pending)
	}
	f.pending = ""
}

func (f *deltaFilter) send(s string) {
	if s != "" {
		f.emit(s)
	}
}

// splitRationale 把最终回答拆成正文和理由；没有标签时整段都是正文。
func splitRationale(final string) (text, rationale string) {
	i := strings.Index(final, rationaleOpen)
	if i < 0 {
		return strings.TrimSpace(final), ""
	}
	text = strings.TrimSpace(final[:i])
	rest := final[i+len(rationaleOpen):]
	if j := strings.Index(rest, "</rationale>"); j >= 0 {
		rest = rest[:j]
	}
	return text, strings.TrimSpace(rest)
}
