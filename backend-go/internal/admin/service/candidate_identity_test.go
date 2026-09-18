package service

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
)

// ── improve-discovery-recommendations 切片 2.1：候选身份纯函数白盒测试 ──
//
// 判据来源 test-cases-foundation.md：URL 规范化（大小写/端口/fragment/query 顺序/userinfo/
// 无 host/非法 scheme/499-501 rune）、名称 199-201、描述 3999-4001、language/region ≤50、
// 全角空白/tab 拒绝、人工覆盖与回退、稳定键不含时间与自增 ID。纯逻辑，不依赖 DB。

func TestBuildRSSHubStableKey(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		path      string
		want      string
		wantErr   string
	}{
		{name: "namespace 与 path 整体小写", namespace: "Bilibili", path: "User/Videos/:id", want: "bilibili/user/videos/:id"},
		{name: "去首尾空白", namespace: "  ifanr  ", path: "  group/:id  ", want: "ifanr/group/:id"},
		{name: "空 namespace 拒绝", namespace: "", path: "user/:id", wantErr: "namespace"},
		{name: "纯空白 namespace 拒绝（含全角）", namespace: " \u3000\t", path: "user/:id", wantErr: "namespace"},
		{name: "空 path 拒绝", namespace: "bilibili", path: "", wantErr: "path"},
		{name: "纯空白 path 拒绝", namespace: "bilibili", path: "  ", wantErr: "path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildRSSHubStableKey(tt.namespace, tt.path)
			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}

	t.Run("大小写变体稳定身份相同", func(t *testing.T) {
		upper, err := BuildRSSHubStableKey("BILIBILI", "USER/:id")
		require.NoError(t, err)
		lower, err := BuildRSSHubStableKey("bilibili", "user/:id")
		require.NoError(t, err)
		require.Equal(t, lower, upper)
	})

	t.Run("同输入重复结果相同（幂等，不含时间/自增 ID）", func(t *testing.T) {
		first, err := BuildRSSHubStableKey("bilibili", "user/:id")
		require.NoError(t, err)
		second, err := BuildRSSHubStableKey("bilibili", "user/:id")
		require.NoError(t, err)
		require.Equal(t, first, second)
		require.Equal(t, "bilibili/user/:id", first) // 确定性字面量：纯 ns/path 组合
	})
}

func TestNormalizeRSSURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		// 大小写 / 默认端口 / fragment / query 顺序（path 大小写保留）
		{name: "scheme host 小写+默认端口+fragment 移除+query 顺序保留", raw: "HTTPS://Blog.EXAMPLE.com:443/Feed/A?b=2&a=1#frag", want: "https://blog.example.com/Feed/A?b=2&a=1"},
		{name: "http 默认端口 80 移除", raw: "http://Example.com:80/x", want: "http://example.com/x"},
		{name: "https 上的 80 非默认端口保留", raw: "https://example.com:80/x", want: "https://example.com:80/x"},
		{name: "非默认端口 8080 保留", raw: "http://example.com:8080/x", want: "http://example.com:8080/x"},
		{name: "首尾空白去除", raw: "  http://example.com/f  ", want: "http://example.com/f"},
		{name: "path 转义与 query 编码原样保留", raw: "http://example.com/A%20B?x=%E4%B8%AD", want: "http://example.com/A%20B?x=%E4%B8%AD"},
		{name: "IPv6 host 小写+默认端口移除", raw: "http://[2001:DB8::1]:80/x", want: "http://[2001:db8::1]/x"},
		// 非法输入
		{name: "userinfo 拒绝", raw: "http://user@example.com/f", wantErr: "userinfo"},
		{name: "带密码 userinfo 拒绝", raw: "https://u:p@example.com/f", wantErr: "userinfo"},
		{name: "无 scheme 相对地址拒绝", raw: "example.com/feed", wantErr: "scheme"},
		{name: "相对路径拒绝", raw: "/feed", wantErr: "scheme"},
		{name: "非 http scheme 拒绝", raw: "ftp://example.com/f", wantErr: "scheme"},
		{name: "file scheme 拒绝", raw: "file:///etc/passwd", wantErr: "scheme"},
		{name: "空 host 拒绝", raw: "http:///feed", wantErr: "host"},
		{name: "非数字端口拒绝", raw: "http://example.com:abc/f", wantErr: "port"},
		{name: "端口超 65535 拒绝", raw: "http://example.com:99999/f", wantErr: "port"},
		{name: "端口 0 拒绝", raw: "http://example.com:0/f", wantErr: "port"},
		{name: "空串拒绝", raw: "", wantErr: "empty"},
		{name: "纯空白拒绝（空格/tab/全角）", raw: " \t\u3000\n", wantErr: "empty"},
		// 499/500/501 rune 边界
		{name: "499 rune 接受", raw: "http://example.com/" + strings.Repeat("a", 480), want: "http://example.com/" + strings.Repeat("a", 480)},
		{name: "500 rune 接受", raw: "http://example.com/" + strings.Repeat("a", 481), want: "http://example.com/" + strings.Repeat("a", 481)},
		{name: "501 rune 拒绝", raw: "http://example.com/" + strings.Repeat("a", 482), wantErr: "exceeds"},
		{name: "中文输入按 rune 计数 500 接受（path 规范转义后身份一致）", raw: "https://example.com/" + strings.Repeat("中", 480), want: "https://example.com/" + strings.Repeat("%E4%B8%AD", 480)},
		{name: "中文按 rune 计数 501 拒绝", raw: "https://example.com/" + strings.Repeat("中", 481), wantErr: "exceeds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeRSSURL(tt.raw)
			if tt.wantErr != "" {
				require.Error(t, err)
				if tt.wantErr != "*" {
					require.Contains(t, err.Error(), tt.wantErr)
				}
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			require.Equal(t, tt.want, *got)
		})
	}

	t.Run("仅 fragment 差异的地址规范身份相同", func(t *testing.T) {
		withFrag, err := NormalizeRSSURL("http://example.com/f#section")
		require.NoError(t, err)
		withoutFrag, err := NormalizeRSSURL("http://example.com/f")
		require.NoError(t, err)
		require.Equal(t, *withoutFrag, *withFrag)
	})
}

func TestBuildRSSStableKey(t *testing.T) {
	rssKeyPattern := regexp.MustCompile(`^rss:[0-9a-f]{64}$`)

	t.Run("格式为 rss: 前缀 + sha256 十六进制（不含时间/自增 ID）", func(t *testing.T) {
		key, err := BuildRSSStableKey("http://example.com/feed.xml")
		require.NoError(t, err)
		require.Regexp(t, rssKeyPattern, key)
	})

	t.Run("同输入重复结果相同", func(t *testing.T) {
		first, err := BuildRSSStableKey("http://example.com/feed.xml")
		require.NoError(t, err)
		second, err := BuildRSSStableKey("http://example.com/feed.xml")
		require.NoError(t, err)
		require.Equal(t, first, second)
	})

	t.Run("大小写/默认端口/fragment 变体身份相同", func(t *testing.T) {
		variants := []string{
			"HTTP://Example.com:80/feed.xml",
			"http://example.com/feed.xml#top",
			"http://EXAMPLE.com/feed.xml",
		}
		keys := make([]string, 0, len(variants))
		for _, raw := range variants {
			key, err := BuildRSSStableKey(raw)
			require.NoError(t, err)
			keys = append(keys, key)
		}
		for _, k := range keys[1:] {
			require.Equal(t, keys[0], k)
		}
	})

	t.Run("path 大小写与 query 顺序不同身份不同", func(t *testing.T) {
		base, err := BuildRSSStableKey("http://example.com/F?a=1&b=2")
		require.NoError(t, err)
		querySwapped, err := BuildRSSStableKey("http://example.com/F?b=2&a=1")
		require.NoError(t, err)
		pathLower, err := BuildRSSStableKey("http://example.com/f?a=1&b=2")
		require.NoError(t, err)
		require.NotEqual(t, base, querySwapped)
		require.NotEqual(t, base, pathLower)
	})

	t.Run("非法地址报错", func(t *testing.T) {
		_, err := BuildRSSStableKey("not a url")
		require.Error(t, err)
		_, err = BuildRSSStableKey("ftp://example.com/f")
		require.Error(t, err)
	})
}

func TestCandidateValidateManualField(t *testing.T) {
	repeat := strings.Repeat

	tests := []struct {
		name    string
		field   string
		value   string
		wantErr string // 空串表示期望通过
	}{
		// name ≤200 且非空白
		{name: "名称 199 rune 接受", field: ManualFieldName, value: repeat("a", 199)},
		{name: "名称 200 rune 接受", field: ManualFieldName, value: repeat("a", 200)},
		{name: "名称 201 rune 拒绝", field: ManualFieldName, value: repeat("a", 201), wantErr: "name"},
		{name: "名称中文按 rune 计数 200 接受", field: ManualFieldName, value: repeat("名", 200)},
		{name: "名称中文按 rune 计数 201 拒绝", field: ManualFieldName, value: repeat("名", 201), wantErr: "name"},
		{name: "名称空串拒绝", field: ManualFieldName, value: "", wantErr: "name"},
		{name: "名称空格拒绝", field: ManualFieldName, value: " ", wantErr: "name"},
		{name: "名称 tab 拒绝", field: ManualFieldName, value: "\t", wantErr: "name"},
		{name: "名称全角空白拒绝", field: ManualFieldName, value: "\u3000", wantErr: "name"},
		{name: "名称混合空白拒绝", field: ManualFieldName, value: "\t \u3000", wantErr: "name"},
		{name: "单 token 名称接受", field: ManualFieldName, value: "news"},
		{name: "名称含特殊字符接受（不做 HTML 转义）", field: ManualFieldName, value: "名称<b>&\"x\"</b>"},
		// description ≤4000（空串合法：表示清空覆盖回退上游）
		{name: "描述 3999 rune 接受", field: ManualFieldDescription, value: repeat("d", 3999)},
		{name: "描述 4000 rune 接受", field: ManualFieldDescription, value: repeat("d", 4000)},
		{name: "描述 4001 rune 拒绝", field: ManualFieldDescription, value: repeat("d", 4001), wantErr: "description"},
		{name: "描述中文 4000 rune 接受", field: ManualFieldDescription, value: repeat("述", 4000)},
		{name: "描述空串接受（回退上游）", field: ManualFieldDescription, value: ""},
		// language / region ≤50
		{name: "语言 50 rune 接受", field: ManualFieldLanguage, value: repeat("l", 50)},
		{name: "语言 51 rune 拒绝", field: ManualFieldLanguage, value: repeat("l", 51), wantErr: "language"},
		{name: "地区 50 rune 接受", field: ManualFieldRegion, value: repeat("r", 50)},
		{name: "地区 51 rune 拒绝", field: ManualFieldRegion, value: repeat("r", 51), wantErr: "region"},
		// 未知字段
		{name: "未知字段拒绝", field: "title", value: "x", wantErr: "title"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateManualField(tt.field, tt.value)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}

	t.Run("错误信息不 HTML 转义拼接（不含原始值）", func(t *testing.T) {
		value := "<script>" + repeat("a", 201) + "</script>"
		err := ValidateManualField(ManualFieldName, value)
		require.Error(t, err)
		require.Contains(t, err.Error(), ManualFieldName)
		require.NotContains(t, err.Error(), "&lt;")
		require.NotContains(t, err.Error(), "<script>")
	})
}

func TestEffectiveMetadata(t *testing.T) {
	upstream := [4]string{"上游名", "上游描述", "zh", "CN"}

	t.Run("人工非空全量覆盖上游", func(t *testing.T) {
		manual := models.MetadataMap{
			ManualFieldName:        "人工名",
			ManualFieldDescription: "人工描述",
			ManualFieldLanguage:    "en",
			ManualFieldRegion:      "US",
		}
		got := EffectiveMetadata(manual, upstream[0], upstream[1], upstream[2], upstream[3])
		require.Equal(t, EffectiveMetadataValues{Name: "人工名", Description: "人工描述", Language: "en", Region: "US"}, got)
	})

	t.Run("人工空串回退上游（键存在性不阻断回退）", func(t *testing.T) {
		manual := models.MetadataMap{
			ManualFieldName:        "",
			ManualFieldDescription: "",
			ManualFieldLanguage:    "",
			ManualFieldRegion:      "",
		}
		got := EffectiveMetadata(manual, upstream[0], upstream[1], upstream[2], upstream[3])
		require.Equal(t, EffectiveMetadataValues{Name: upstream[0], Description: upstream[1], Language: upstream[2], Region: upstream[3]}, got)
	})

	t.Run("nil manual 全用上游", func(t *testing.T) {
		got := EffectiveMetadata(nil, upstream[0], upstream[1], upstream[2], upstream[3])
		require.Equal(t, EffectiveMetadataValues{Name: upstream[0], Description: upstream[1], Language: upstream[2], Region: upstream[3]}, got)
	})

	t.Run("部分覆盖只影响对应字段", func(t *testing.T) {
		manual := models.MetadataMap{ManualFieldDescription: "仅覆盖描述"}
		got := EffectiveMetadata(manual, upstream[0], upstream[1], upstream[2], upstream[3])
		require.Equal(t, "仅覆盖描述", got.Description)
		require.Equal(t, upstream[0], got.Name)
		require.Equal(t, upstream[2], got.Language)
		require.Equal(t, upstream[3], got.Region)
	})

	t.Run("人工与上游皆缺不编造资料", func(t *testing.T) {
		got := EffectiveMetadata(models.MetadataMap{}, "", "", "", "")
		require.Equal(t, EffectiveMetadataValues{}, got)
	})

	t.Run("上游为空时人工非空生效", func(t *testing.T) {
		manual := models.MetadataMap{ManualFieldName: "人工名"}
		got := EffectiveMetadata(manual, "", "", "", "")
		require.Equal(t, "人工名", got.Name)
	})

	t.Run("非字符串值视为无覆盖回退上游", func(t *testing.T) {
		manual := models.MetadataMap{ManualFieldName: 42}
		got := EffectiveMetadata(manual, upstream[0], upstream[1], upstream[2], upstream[3])
		require.Equal(t, upstream[0], got.Name)
	})

	// RSSHub 上游 description 是文档页 markdown 源码（表格/链接/<details>/::: tip），
	// 原文直出会在候选库卡片呈现一大坨 URL（2026-09 用户实测）。出口统一清洗：
	// 人工与上游都过 SanitizeEffectiveText，语言/地区原样透传。
	t.Run("上游脏 markdown 出口清洗（链接保留文字/格式噪音去除/空白折叠）", func(t *testing.T) {
		dirty := "::: tip\n若订阅 [行业资讯](https://www.cngold.org.cn/news-325.html) 截取参数\n:::\n| 资讯中心 | [图片新闻](https://www.cngold.org.cn/news-323.html) |\n<details> <summary>更多分类</summary>"
		got := EffectiveMetadata(nil, "[分类](https://example.com/docs) 路由", dirty, "zh", "")
		require.Equal(t, "分类 路由", got.Name)
		require.Equal(t, "若订阅 行业资讯 截取参数 | 资讯中心 | 图片新闻 | 更多分类", got.Description)
		require.Equal(t, "zh", got.Language)
		require.Equal(t, "", got.Region)
	})

	t.Run("人工脏 markdown 同样清洗", func(t *testing.T) {
		manual := models.MetadataMap{
			ManualFieldName:        "**加粗名**",
			ManualFieldDescription: "看 [官方博客](https://example.com/blog) 的更新",
		}
		got := EffectiveMetadata(manual, "", "", "", "")
		require.Equal(t, "加粗名", got.Name)
		require.Equal(t, "看 官方博客 的更新", got.Description)
	})
}
