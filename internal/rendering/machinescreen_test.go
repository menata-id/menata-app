package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

func TestMachineSchemaIsShownToAdminsOnly(t *testing.T) {
	if !showMachineSchema(domain.Actor{WorkspaceRole: "admin"}) || showMachineSchema(domain.Actor{WorkspaceRole: "member"}) {
		t.Fatal("the Machine id and Field schema table are for a Workspace admin only")
	}
}

func TestMachineBody_recordsComeBeforeTheFieldSchema(t *testing.T) {
	m := docMachine()
	records := []*data.Record{{ID: "doc_1", Values: map[string]any{"fld_title": "Kontrak A"}}}
	var buf bytes.Buffer
	if err := MachineBody(m, m.Views[1], records, nil, nil, nil, nil, domain.Actor{ID: "usr_ana", WorkspaceRole: "admin"}, time.Now(), nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	got := buf.String()
	rec, fields := strings.Index(got, "Records ("), strings.Index(got, ">Fields<")
	if rec < 0 || fields < 0 || rec > fields {
		t.Fatalf("records at %d, schema at %d: an admin's schema table must sit below the records", rec, fields)
	}
}
