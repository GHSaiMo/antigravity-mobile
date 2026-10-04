package proxy

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTranslateLiteralContexts(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"positional child", `z.createElement(Btn,{onClick:f},"Cancel")`, `"取消"`},
		{"child after newline", "z.createElement(Btn,{onClick:f},\n    \"Cancel\")", `"取消"`},
		{"child after call", `z.createElement(Icon,{name:"add"}),"Add")`, `"添加"`},
		{"ui property", `{title:"Hooks",count:1}`, `"Hooks"`}, // identical in zh
		{"ternary then", `f(c?"Saving...":x)`, `"保存中..."`},
		{"ternary else", `f(c?"Foo":"Saving...")`, `"保存中..."`},
		{"fallback", `{content:a.tooltipContent??"Stop"}`, `"停止"`},
		{"arrow return", `getTabTitle:()=>"Preview"`, `"预览"`},
		{"default param", `({cancelLabel:l="Cancel"})`, `"取消"`},
		{"sentence as call arg", `toast("Copied to clipboard")`, `"已复制到剪贴板"`},
		{"compare untouched", `if(a==="Cancel"||b==="Saving...")`, `"Cancel"`},
		{"includes untouched", `e.message.includes("Terms of Service")`, `"Terms of Service"`},
		{"enum reverse map untouched", `X[X.Running=1]="Running"`, `"Running"`},
		{"non-ui property key untouched", `{kind:"Cancel"}`, `"Cancel"`},
		{"call arg word untouched", `track("Cancel")`, `"Cancel"`},
	}
	for _, c := range cases {
		out := string(translateStringLiterals([]byte(c.in)))
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: want %s in %q", c.name, c.want, out)
		}
	}
}

func TestTranslateLiteralKeepsNonDictionaryText(t *testing.T) {
	in := []byte(`a="x \" y",b='"Cancel"',c="Unknown Phrase Here"`)
	out := translateStringLiterals(in)
	if string(out) == string(in) {
		return
	}
	// the single-quoted '"Cancel"' is the only possible hit; it is a call-free literal context
	if !strings.Contains(string(out), `c="Unknown Phrase Here"`) {
		t.Fatalf("unrelated literal must be preserved: %q", out)
	}
}

func TestL10nPatches(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"thought for", "`Thought for ${Math.max(1,Number(b.seconds))}s`", "`已思考 ${Math.max(1,Number(b.seconds))} 秒`"},
		{"worked for", "`Worked for ${HK(a)}`", "`已工作 ${HK(a)}`"},
		{"see all", "`See all (${c})`", "`查看全部 (${c})`"},
		{"relative time suffix", `c.comparison>0?"in "+b:b+" ago"`, `c.comparison>0?b+"后":b+"前"`},
		{"date-fns minutes", `xMinutes:{one:"1 minute",other:"{{count}} minutes"}`, `{one:"1 分钟",other:"{{count}} 分钟"}`},
		{"thinking for", `z.createElement(z.Fragment,null,"Thinking for ",Math.max(1,b),"s")`, `"思考 ",Math.max(1,b),"秒"`},
		{"running/ran", `f=c?g?"Running":"running":g?"Ran":"ran"`, `f=c?g?"正在运行":"正在运行":g?"已运行":"已运行"`},
		{"files changed", "`${u} ${u===1?\"file\":\"files\"} changed`", "`${u} 个文件已更改`"},
	}
	for _, c := range cases {
		out := string(LocalizeMainJS([]byte(c.in)))
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: want %q in %q", c.name, c.want, out)
		}
	}
}

// Translations are spliced into JS string literals (and template literals), so a stray quote,
// backslash, backtick or line break would corrupt the whole 9MB bundle.
func TestL10nDictionariesAreSpliceSafe(t *testing.T) {
	bad := func(v string) bool { return strings.ContainsAny(v, "\"\\`\n\r") || strings.Contains(v, "${") }
	for name, m := range map[string]map[string]string{
		"l10nAnywhere": l10nAnywhere, "l10nUIProp": l10nUIProp, "l10nHeaderProp": l10nHeaderProp,
		"l10nSafe": l10nSafe, "l10nTern": l10nTern, "l10nStrict": l10nStrict,
	} {
		for k, v := range m {
			if k == "" || v == "" {
				t.Errorf("%s: empty key or value for %q", name, k)
			}
			if bad(v) {
				t.Errorf("%s[%q] = %q contains a character unsafe inside a JS literal", name, k, v)
			}
		}
	}
	for _, p := range l10nPatches {
		if strings.ContainsAny(p.repl, "\n\r") {
			t.Errorf("patch %q replacement contains a line break", p.anchor)
		}
	}
}

func TestLocalizeMainJSIsFast(t *testing.T) {
	// ~2MB of filler plus a few literals: a linear pass must stay far below the former
	// per-rule ReplaceAll implementation (~10s for a 9.6MB bundle).
	filler := strings.Repeat(`var a=z.createElement("div",{className:"x"},b,"hello world"),c=f("some text");`, 25000)
	in := []byte(filler + `z.createElement(Btn,{},"Cancel")`)
	out := LocalizeMainJS(in)
	if !strings.HasSuffix(string(out), `"取消")`) {
		t.Fatalf("tail literal not translated")
	}
}

// TestLocalizeRealBundle is a developer aid: point L10N_MAIN_JS at a main.js fetched from the
// language server (curl -sk -H "x-codeium-csrf-token: ..." https://127.0.0.1:PORT/main.js) to
// check that the dictionaries still match the upstream bundle after an IDE upgrade.
// L10N_OUT, when set, receives the translated bundle (e.g. for `node --check`).
func TestLocalizeRealBundle(t *testing.T) {
	path := os.Getenv("L10N_MAIN_JS")
	if path == "" {
		t.Skip("set L10N_MAIN_JS to a real main.js to run")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	out := LocalizeMainJS(data)
	t.Logf("localized %d -> %d bytes in %v", len(data), len(out), time.Since(start))
	if bytes.Equal(out, data) {
		t.Fatal("no rule matched the bundle; upstream may have changed")
	}
	for _, want := range []string{"已思考 ", `"后"`, "新建会话"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("expected translated bundle to contain %q", want)
		}
	}
	if o := os.Getenv("L10N_OUT"); o != "" {
		if err := os.WriteFile(o, out, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLocalizeRPCResponse(t *testing.T) {
	in := []byte(`{"response":{"groups":[{"displayName":"Gemini Models", "description":"Models within this group: Gemini Flash, Gemini Pro", "buckets":[{"displayName":"Weekly Limit Remaining", "description":"You have used some of your weekly limit, it will fully refresh in 3 days, 17 hours.", "window":"weekly"},{"displayName":"Five Hour Limit Remaining","description":"You have used some of your 5-hour limit, it will fully refresh in 4 hours."}]}]},"label":"Gemini 3.1 Pro (High)","tagTitle":"Notice","tagDescription":"Sonnet 5.5 is now available on paid Pro and Ultra plans. Third-party model access will no longer be available on your current plan starting on November 2, 2026.","upgradeSubscriptionText":"You can upgrade to a Google AI Ultra plan to receive higher rate limits.","name":"Recommended"}`)
	out := string(LocalizeRPCResponse("/exa.language_server_pb.LanguageServerService/RetrieveUserQuotaSummary", in))
	for _, want := range []string{
		`"displayName":"Gemini 模型群"`, `"每周剩余额度"`, `"5小时剩余额度"`, `将在 3 天 17 小时 后完全刷新`, `将在 4 小时 后完全刷新`,
		`"tagTitle":"提示"`, `自 2026 年 11 月 2 日起`, `该分组包含的模型：Gemini Flash, Gemini Pro`,
		`"label":"Gemini 3.1 Pro (High)"`, // model labels are identifiers: untouched
	} {
		if !strings.Contains(out, want) {
			t.Errorf("want %s in %s", want, out)
		}
	}
	for _, keep := range []string{`"name":"Recommended"`, `"upgradeSubscriptionText":"You can upgrade`} {
		if !strings.Contains(out, keep) {
			t.Errorf("%s is read by the workbench and must stay untranslated: %s", keep, out)
		}
	}
	if got := LocalizeRPCResponse("/x/GetCascadeTrajectory", in); string(got) != string(in) {
		t.Error("non-allowlisted RPC must not be rewritten")
	}
}

func TestSettingsNavAndQuotaPatches(t *testing.T) {
	in := `const r6a=new Map(i6a.filter(a=>a.label!==void 0).map(a=>[a.screen,a.label]));function s6a(a){return r6a.get(a)??a};x=` + "`Resets in ${e}d`"
	out := string(LocalizeMainJS([]byte(in)))
	for _, want := range []string{`r6a.get(a)??{Account:"账户与计划"`, `[a]??a}`, "`距重置 ${e}d`"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in %q", want, out)
		}
	}
}
