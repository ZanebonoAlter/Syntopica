package service

import "testing"

func TestParseContentFormMark(t *testing.T) {
	cases := []struct {
		name      string
		summary   string
		wantForm  string
		wantClean string
	}{
		{
			name:      "mono mark on first line",
			summary:   "<!-- form: mono -->\n# 教程\n\n正文内容",
			wantForm:  "mono",
			wantClean: "# 教程\n\n正文内容",
		},
		{
			name:      "aggregate mark on first line",
			summary:   "<!-- form: aggregate -->\n# 周刊\n\n## 导读\n- 条目",
			wantForm:  "aggregate",
			wantClean: "# 周刊\n\n## 导读\n- 条目",
		},
		{
			name:      "no mark keeps original summary",
			summary:   "# 无标记文章\n\n正文",
			wantForm:  "",
			wantClean: "# 无标记文章\n\n正文",
		},
		{
			name:      "mark in middle of content is not stripped",
			summary:   "# 标题\n\n<!-- form: mono -->\n\n正文",
			wantForm:  "",
			wantClean: "# 标题\n\n<!-- form: mono -->\n\n正文",
		},
		{
			name:      "empty summary",
			summary:   "",
			wantForm:  "",
			wantClean: "",
		},
		{
			name:      "invalid form value treated as no mark",
			summary:   "<!-- form: hybrid -->\n# 标题\n\n正文",
			wantForm:  "",
			wantClean: "<!-- form: hybrid -->\n# 标题\n\n正文",
		},
		{
			name:      "whitespace around first line mark",
			summary:   "  <!-- form: aggregate -->  \n\n# 周刊\n\n正文",
			wantForm:  "aggregate",
			wantClean: "# 周刊\n\n正文",
		},
		{
			name:      "mark only leaves empty summary",
			summary:   "<!-- form: mono -->",
			wantForm:  "mono",
			wantClean: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotForm, gotClean := parseContentFormMark(tc.summary)
			if gotForm != tc.wantForm {
				t.Fatalf("form = %q, want %q", gotForm, tc.wantForm)
			}
			if gotClean != tc.wantClean {
				t.Fatalf("cleaned = %q, want %q", gotClean, tc.wantClean)
			}
		})
	}
}
