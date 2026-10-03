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

