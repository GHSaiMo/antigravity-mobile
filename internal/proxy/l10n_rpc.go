package proxy

import (
	"bytes"
	"encoding/binary"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Localization of server-sent display text. The language server returns quota groups,
// plan notices and upgrade hints as English strings inside unary JSON RPC responses, so
// they never appear in main.js. They are translated here, in the gateway, before the
// browser sees them.
//
// Only an allowlist of RPCs and display-only JSON fields are touched, and a value is
// replaced only when it matches a known phrase/pattern, so conversation content and
// anything the workbench uses as a key or parses is never rewritten. In particular NOT
// translated: model labels ("Gemini 3.1 Pro (High)"), group "name" ("Recommended" is looked
// up by name in the model selector - translating it broke the Models page and account
// menu), and userTier upgradeSubscriptionText/upgradeButtonText (fed into 429 handling).

// Only the quota summary is translated. GetUserStatus / GetCascadeModelConfigData feed the
// workbench's userStatus state (account menu, model selector, auth redirect); rewriting
// them broke the account button and the Models page, so they are passed through verbatim.
var l10nRPCSuffixes = []string{"/RetrieveUserQuotaSummary"}

// l10nDisabled reports whether the MGY_L10N_DISABLE kill switch covers part ("rpc" or
// "bundle"); the value "all" (or any other non-empty value) disables both.
func l10nDisabled(part string) bool {
	v := os.Getenv("MGY_L10N_DISABLE")
	return v != "" && (v == part || v == "all" || (v != "rpc" && v != "bundle"))
}

func isL10nRPCPath(path string) bool {
	if l10nDisabled("rpc") {
		return false
	}
	for _, s := range l10nRPCSuffixes {
		if strings.HasSuffix(path, s) {
			return true
		}
	}
	return false
}

var reRPCTextField = regexp.MustCompile(`("(?:displayName|description|tagTitle|tagDescription)"\s*:\s*")((?:[^"\\]|\\.)*)(")`)

var l10nRPCExact = map[string]string{
	"Gemini Models":             "Gemini 模型群",
	"Claude and GPT models":     "Claude 与 GPT 模型群",
	"Weekly Limit Remaining":    "每周剩余额度",
	"Five Hour Limit Remaining": "5小时剩余额度",
	"Notice":                    "提示",
	"Within each group, models share a weekly limit and a 5-hour limit. Quota is consumed proportionally to the cost of the tokens. Thus, limits will last longer with shorter tasks or using more cost-effective models. The 5-hour limit smooths out aggregate demand to fairly distribute global capacity across all users, while your weekly limit is tied directly to your individual tier.": "同一分组内的模型共享每周额度与 5 小时额度。额度按 Token 成本等比例消耗，因此任务越短或使用越经济的模型，额度可用越久。5 小时额度用于平滑整体需求、在所有用户间公平分配全球算力，而每周额度则与您的个人套餐直接挂钩。",
}

var (
	reQuotaUsed   = regexp.MustCompile(`^You have used (some|all) of your (weekly|5-hour) limit, it will fully refresh in (.+)\.$`)
	reQuotaGroup  = regexp.MustCompile(`^Models within this group: (.+)$`)
	reModelAvail  = regexp.MustCompile(`^(.+) is now available on paid Pro and Ultra plans\. Third-party model access will no longer be available on your current plan starting on (.+)\.$`)
	reModelRemove = regexp.MustCompile(`^(.+) will be removed from Antigravity on (.+)\.$`)
	reEnDate      = regexp.MustCompile(`^([A-Z][a-z]+) (\d{1,2}), (\d{4})$`)
	reDurUnit     = regexp.MustCompile(`(\d+) (day|hour|minute|second)s?`)
)

var enMonths = map[string]string{
	"January": "1", "February": "2", "March": "3", "April": "4", "May": "5", "June": "6",
	"July": "7", "August": "8", "September": "9", "October": "10", "November": "11", "December": "12",
}

var zhDurUnit = map[string]string{"day": "天", "hour": "小时", "minute": "分钟", "second": "秒"}

// zhDuration turns "3 days, 17 hours" into "3 天 17 小时".
func zhDuration(s string) string {
	s = reDurUnit.ReplaceAllStringFunc(s, func(m string) string {
		sub := reDurUnit.FindStringSubmatch(m)
		return sub[1] + " " + zhDurUnit[sub[2]]
	})
	return strings.NewReplacer(", and ", " ", ", ", " ", " and ", " ").Replace(s)
}

func zhDate(s string) string {
	if m := reEnDate.FindStringSubmatch(s); m != nil {
		if mon, ok := enMonths[m[1]]; ok {
			return m[3] + " 年 " + mon + " 月 " + m[2] + " 日"
		}
	}
	return s
}

// translateRPCText returns the translation of one display string, or ("", false).
func translateRPCText(s string) (string, bool) {
	if zh, ok := l10nRPCExact[s]; ok {
		return zh, true
	}
	if m := reQuotaUsed.FindStringSubmatch(s); m != nil {
		amount := map[string]string{"some": "部分", "all": "全部"}[m[1]]
		kind := map[string]string{"weekly": "每周", "5-hour": "5 小时"}[m[2]]
		return "您已使用" + amount + " " + kind + "额度，将在 " + zhDuration(m[3]) + " 后完全刷新。", true
	}
	if m := reQuotaGroup.FindStringSubmatch(s); m != nil {
		return "该分组包含的模型：" + m[1], true
	}
	if m := reModelAvail.FindStringSubmatch(s); m != nil {
		return m[1] + " 现已面向付费 Pro 和 Ultra 套餐开放。自 " + zhDate(m[2]) + "起，您当前的套餐将无法再使用第三方模型。", true
	}
	if m := reModelRemove.FindStringSubmatch(s); m != nil {
		return m[1] + " 将于 " + zhDate(m[2]) + "从 Antigravity 中移除。", true
	}
	return "", false
}

// LocalizeRPCResponse translates the display strings of an allowlisted unary JSON RPC response.
func LocalizeRPCResponse(path string, body []byte) []byte {
	if !isL10nRPCPath(path) || len(body) == 0 || len(body) > 1<<20 {
		return body
	}
	return reRPCTextField.ReplaceAllFunc(body, func(m []byte) []byte {
		sub := reRPCTextField.FindSubmatch(m)
		zh, ok := translateRPCText(string(sub[2]))
		if !ok {
			return m
		}
		return []byte(string(sub[1]) + zh + string(sub[3]))
	})
}

// localizeRPCResponse rewrites resp.Body in place with translated display strings.
func localizeRPCResponse(resp *http.Response) error {
	body := resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, err := GetGzipReader(resp.Body)
		if err != nil {
			return err
		}
		body = &pooledGzipReadCloser{gz: gz, body: resp.Body}
		resp.Header.Del("Content-Encoding")
	}
	raw, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil {
		return err
	}
	var out []byte
	if ct := resp.Header.Get("Content-Type"); strings.Contains(ct, "connect+") || strings.Contains(ct, "grpc") {
		out = localizeConnectEnvelopes(resp.Request.URL.Path, raw)
	} else {
		out = LocalizeRPCResponse(resp.Request.URL.Path, raw)
	}
	resp.Body = io.NopCloser(bytes.NewReader(out))
	resp.ContentLength = int64(len(out))
	resp.Header.Set("Content-Length", strconv.Itoa(len(out)))
	return nil
}

// localizeConnectEnvelopes translates an enveloped body (Connect streaming / grpc-web, e.g.
// application/grpc-web+json):
// a sequence of frames [flags:1][length:4 big-endian][payload]. Each payload is rewritten
// and its length prefix recomputed; a malformed body is returned untouched.
func localizeConnectEnvelopes(path string, body []byte) []byte {
	var out []byte
	for pos := 0; pos < len(body); {
		if len(body)-pos < 5 {
			return body
		}
		n := int(binary.BigEndian.Uint32(body[pos+1 : pos+5]))
		if n < 0 || pos+5+n > len(body) {
			return body
		}
		payload := body[pos+5 : pos+5+n]
		// only plain data frames (flags 0); compressed frames and the grpc-web/Connect trailer
		// frame (flag 0x80 / 0x02) are passed through
		if body[pos] == 0 {
			payload = LocalizeRPCResponse(path, payload)
		}
		var hdr [5]byte
		hdr[0] = body[pos]
		binary.BigEndian.PutUint32(hdr[1:], uint32(len(payload)))
		out = append(out, hdr[:]...)
		out = append(out, payload...)
		pos += 5 + n
	}
	return out
}
