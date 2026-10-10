package rendering

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func TestFoldHeadAndTailSplitAtN(t *testing.T) {
	rows := []int{1, 2, 3, 4, 5, 6, 7}
	if h, tl := foldHead(rows, 5), foldTail(rows, 5); len(h) != 5 || len(tl) != 2 || tl[0] != 6 {
		t.Errorf("split = %v | %v", h, tl)
	}
	if h, tl := foldHead(rows[:5], 5), foldTail(rows[:5], 5); len(h) != 5 || len(tl) != 0 {
		t.Errorf("a list of exactly n folds nothing: %v | %v", h, tl)
	}
}

func TestChecklistSection_FoldsRowsPastFiveBehindShowAll(t *testing.T) {
	m := &domain.Machine{ID: "mch_checklist_item", Name: "Checklist"}
	c := &Checklist{ParentField: "fld_task", TextField: "fld_text"}
	for i := 0; i < 8; i++ {
		c.Items = append(c.Items, ChecklistItem{ID: fmt.Sprint("i", i), Text: fmt.Sprint("Item number ", i)})
	}
	var buf strings.Builder
	if err := checklistSection(m, "tsk_1", c, true).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	head, folded, ok := strings.Cut(out, "<details")
	if !ok || !strings.Contains(out, "Show all 8 items") {
		t.Fatalf("8 items must fold behind \"Show all 8 items\"\n%s", out)
	}
	if strings.Contains(head, "Item number 5") || !strings.Contains(head, "Item number 4") {
		t.Errorf("the first five stay in view and the sixth does not")
	}
	if !strings.Contains(folded, "Item number 7") {
		t.Errorf("the rest sit inside the fold")
	}

	c.Items = c.Items[:5]
	buf.Reset()
	if err := checklistSection(m, "tsk_1", c, true).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "<details") {
		t.Errorf("five items draw no fold")
	}
}

func TestAttachmentsSection_FoldsRowsPastFiveBehindShowAll(t *testing.T) {
	a := &Attachments{ParentField: "fld_task", FileField: "fld_file"}
	for i := 0; i < 6; i++ {
		a.Items = append(a.Items, AttachmentItem{ID: fmt.Sprint("a", i), Key: fmt.Sprint("k", i), Name: fmt.Sprint("file", i, ".pdf"), Kind: "PDF"})
	}
	m := &domain.Machine{ID: "mch_attachment", Name: "Attachments"}
	var buf strings.Builder
	if err := attachmentsSection(m, "tsk_1", a, true).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	head, folded, ok := strings.Cut(out, "<details")
	if !ok || !strings.Contains(out, "Show all 6 attachments") {
		t.Fatalf("6 attachments must fold\n%s", out)
	}
	if strings.Contains(head, "file5.pdf") || !strings.Contains(folded, "file5.pdf") {
		t.Errorf("only the sixth sits inside the fold")
	}
}

func TestCommentsFeed_FoldsEarlierActivityPastTheNewestThree(t *testing.T) {
	var entries []ActivityEntry
	for i := 0; i < 5; i++ {
		entries = append(entries, ActivityEntry{Summary: fmt.Sprint("said number ", i), Actor: "Rina", When: "2026-10-0" + fmt.Sprint(9-i), Comment: true})
	}
	var buf strings.Builder
	if err := commentsFeed("tsk_1", RecordExtras{Activity: entries}, false).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	head, folded, ok := strings.Cut(out, "<details")
	if !ok || !strings.Contains(out, "Show earlier activity (2)") {
		t.Fatalf("5 entries must fold the last 2\n%s", out)
	}
	if !strings.Contains(head, "said number 2") || strings.Contains(head, "said number 3") || !strings.Contains(folded, "said number 4") {
		t.Errorf("the newest three stay in view, the older two fold")
	}
}

func TestLongText_ReadMoreOnlyWhenLong(t *testing.T) {
	var buf strings.Builder
	if err := longText("short note").Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "<details") {
		t.Errorf("a short text draws no fold")
	}
	long := strings.Repeat("é", longTextFold+20)
	buf.Reset()
	if err := longText(long).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "<details") || !strings.Contains(out, "Read more") || !strings.Contains(out, long) {
		t.Errorf("a long text folds behind Read more and still carries the whole text\n%s", out)
	}
	if strings.Contains(out, "�") {
		t.Errorf("the opening is cut on a rune boundary")
	}
}
