package proxy

import "testing"

func TestLocalizeMainJSCheckedPassesThroughAndTranslates(t *testing.T) {
	in := []byte(`x="Thinking..."`)
	out := localizeMainJSChecked(in)
	if string(out) == string(in) {
		t.Fatalf("expected translation, got unchanged output")
	}
	unrelated := []byte(`var a=1;`)
	if got := localizeMainJSChecked(unrelated); string(got) != string(unrelated) {
		t.Fatalf("unrelated input must be unchanged")
	}
}
