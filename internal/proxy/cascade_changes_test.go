package proxy

import "testing"

func TestInvertRevertPreviewCreatedFile(t *testing.T) {
	// 撤销时要删除整个文件 = Agent 新建了该文件
	files := invertRevertPreview([]RevertPreviewFile{{
		FileURI: "file:///w/SKILL.md", FileName: "SKILL.md", ActionType: "DELETE",
		Deletions: 2,
		DiffLines: []RevertDiffLine{{Text: "a", Type: "DELETE"}, {Text: "b", Type: "DELETE"}, {Text: "", Type: "UNCHANGED"}},
	}})
	if len(files) != 1 {
		t.Fatalf("got %d files", len(files))
	}
	f := files[0]
	if f.ActionType != "CREATE" || f.Additions != 2 || f.Deletions != 0 {
		t.Fatalf("unexpected: %+v", f)
	}
	for _, l := range f.DiffLines[:2] {
		if l.Type != "INSERT" {
			t.Errorf("line %q should be INSERT, got %s", l.Text, l.Type)
		}
	}
	if f.DiffLines[2].Type != "UNCHANGED" {
		t.Error("unchanged line must stay unchanged")
	}
}

func TestInvertRevertPreviewModifiedAndDeleted(t *testing.T) {
	files := invertRevertPreview([]RevertPreviewFile{
		{
			FileName: "m.go", ActionType: "MODIFY", Additions: 1, Deletions: 1,
			// 撤销 diff：删掉 Agent 写的 "new"，补回原来的 "old"
			DiffLines: []RevertDiffLine{
				{Text: "ctx", Type: "UNCHANGED"},
				{Text: "new", Type: "DELETE"},
				{Text: "old", Type: "INSERT"},
				{Text: "ctx2", Type: "UNCHANGED"},
			},
		},
		{FileName: "gone.txt", ActionType: "CREATE", Additions: 3},
	})

	m := files[0]
	if m.ActionType != "MODIFY" || m.Additions != 1 || m.Deletions != 1 {
		t.Fatalf("modify: %+v", m)
	}
	// 正向 diff：先删原来的 old，再加新的 new
	want := []RevertDiffLine{{"ctx", "UNCHANGED"}, {"old", "DELETE"}, {"new", "INSERT"}, {"ctx2", "UNCHANGED"}}
	for i, w := range want {
		if m.DiffLines[i] != w {
			t.Errorf("line %d = %+v, want %+v", i, m.DiffLines[i], w)
		}
	}

	d := files[1]
	if d.ActionType != "DELETE" || d.Deletions != 3 || d.Additions != 0 {
		t.Fatalf("deleted: %+v", d)
	}
}

func TestDeletesBeforeInsertsKeepsSeparateBlocks(t *testing.T) {
	in := []RevertDiffLine{
		{"i1", "INSERT"}, {"d1", "DELETE"}, {"c", "UNCHANGED"},
		{"i2", "INSERT"}, {"d2", "DELETE"},
	}
	got := deletesBeforeInserts(in)
	want := []string{"d1", "i1", "c", "d2", "i2"}
	for i, w := range want {
		if got[i].Text != w {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestInvertRevertPreviewEmpty(t *testing.T) {
	if got := invertRevertPreview(nil); got == nil || len(got) != 0 {
		t.Fatalf("empty input must give empty non-nil slice, got %#v", got)
	}
}
