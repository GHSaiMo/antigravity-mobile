package proxy

import (
	"bytes"
	"testing"
)

func TestLocalizeMainJS(t *testing.T) {
	input := []byte(`const a = "New Conversation"; const b = { text: "Settings", tooltip:"Projects" }; const c = "Ask anything, @ to mention" + ", / for actions"; const d = "No Model Selected"; const e = "@import url('https://fonts.googleapis.com/css2');";`)
	output := LocalizeMainJS(input)

	if !bytes.Contains(output, []byte(`"新建会话"`)) {
		t.Errorf("expected output to contain '新建会话', got: %s", string(output))
	}
	if !bytes.Contains(output, []byte(`text: "系统设置"`)) {
		t.Errorf("expected output to contain 'text: \"系统设置\"', got: %s", string(output))
	}
	if !bytes.Contains(output, []byte(`tooltip:"工程项目"`)) {
		t.Errorf("expected output to contain 'tooltip:\"工程项目\"', got: %s", string(output))
	}
	if !bytes.Contains(output, []byte(`"输入任意内容，@ 提及"`)) {
		t.Errorf("expected output to contain '输入任意内容，@ 提及', got: %s", string(output))
	}
	if !bytes.Contains(output, []byte(`"，/ 触发指令"`)) {
		t.Errorf("expected output to contain '，/ 触发指令', got: %s", string(output))
	}
	if !bytes.Contains(output, []byte(`"未选择模型"`)) {
		t.Errorf("expected output to contain '未选择模型', got: %s", string(output))
	}
	if bytes.Contains(output, []byte(`@import url('https://fonts.googleapis.com/`)) {
		t.Errorf("expected fonts.googleapis.com to be stripped from output, got: %s", string(output))
	}
}

func TestLocalizeMainJS_DisposeGC(t *testing.T) {
	input := []byte(`_scheduleGc(a,b){b.gcTimer!==void 0&&clearTimeout(b.gcTimer);b.gcTimer=setTimeout(()=>{this.JSC$7815__states.get(a)===b&&b.holds<=0&&this._disposeEntry(a)},3E4)}`)
	output := LocalizeMainJS(input)
	if !bytes.Contains(output, []byte(`this._disposeEntry(a)},0)`)) {
		t.Fatalf("expected output to contain this._disposeEntry(a)},0), got: %s", string(output))
	}
	if bytes.Contains(output, []byte(`3E4`)) {
		t.Fatalf("expected 3E4 to be replaced with 0, got: %s", string(output))
	}
}

func TestLocalizeMainJS_DisposeGC_DifferentVar(t *testing.T) {
	input := []byte(`_scheduleGc(x,y){y.gcTimer!==void 0&&clearTimeout(y.gcTimer);y.gcTimer=setTimeout(()=>{this._states.get(x)===y&&y.holds<=0&&this._disposeEntry(x)},30000)}`)
	output := LocalizeMainJS(input)
	if !bytes.Contains(output, []byte(`this._disposeEntry(x)},0)`)) {
		t.Fatalf("expected output to contain this._disposeEntry(x)},0), got: %s", string(output))
	}
	if bytes.Contains(output, []byte(`30000`)) {
		t.Fatalf("expected 30000 to be replaced with 0, got: %s", string(output))
	}
}

func TestLocalizeMainJS_DisableOnboarding(t *testing.T) {
	input := []byte(`const a = "features:{onboarding:{feature:{enabled:!0,screens:[7]}}}"; const b = "onboarding:{feature:{enabled:!0,screens:[2,\n7,1,8]}"; const c = "q=!p?.length||p.includes(2);"; const d = "hasOnboardingScreens:f,";`)
	output := LocalizeMainJS(input)

	if bytes.Contains(output, []byte(`q=!p?.length||p.includes(2)`)) {
		t.Errorf("expected q=!p?.length||p.includes(2) to be replaced, got: %s", string(output))
	}
	if !bytes.Contains(output, []byte(`q=!1;`)) {
		t.Errorf("expected q=!1;, got: %s", string(output))
	}
	if bytes.Contains(output, []byte(`hasOnboardingScreens:f`)) {
		t.Errorf("expected hasOnboardingScreens:f to be replaced, got: %s", string(output))
	}
	if !bytes.Contains(output, []byte(`hasOnboardingScreens:!1,`)) {
		t.Errorf("expected hasOnboardingScreens:!1,, got: %s", string(output))
	}
	if bytes.Contains(output, []byte(`onboarding:{feature:{enabled:!0,screens:[7]}`)) {
		t.Errorf("expected onboarding screens:[7] to be disabled, got: %s", string(output))
	}
	if bytes.Contains(output, []byte(`onboarding:{feature:{enabled:!0,screens:[2,\n7,1,8]}`)) {
		t.Errorf("expected onboarding screens:[2,7,1,8] to be disabled, got: %s", string(output))
	}
	if !bytes.Contains(output, []byte(`onboarding:{feature:{enabled:!1,screens:[]}`)) {
		t.Errorf("expected onboarding to have enabled:!1, got: %s", string(output))
	}
}

