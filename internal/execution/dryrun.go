package execution

import (
	"context"
	"fmt"
	"strings"
	"time"

	"menata.app/internal/behavior"
	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// DryRun says what a rule would do for one real record, without doing it: it writes no record, sends no mail and changes no Field. It reads only what the rule itself would read
// (the record, the people it would tell), so what it says is what the rule does, in words, and the Application can be tried before a person is told anything.
//
// A rule that runs on a change cannot be asked "did it happen?" of a record that is not changing, so it is asked "if it did": the line says when the rule runs and where this record
// stands against that. A rule that runs on a date is judged against now.
func DryRun(ctx context.Context, store *data.Store, machines map[string]*domain.Machine, machine *domain.Machine, record *data.Record, event domain.Event, now time.Time) []string {
	name := func(id string) string {
		if f, ok := machine.FieldByID(id); ok {
			return f.Name
		}
		return id
	}
	var lines []string
	switch {
	case event.Disabled:
		return []string{"This rule is switched off, so it does not run."}
	case event.Schedule != nil:
		matched := false
		for _, m := range behavior.MatchedScheduleEvents(machine, record, now) {
			matched = matched || m.ID == event.ID
		}
		if matched {
			lines = append(lines, "It would run now: "+name(event.Schedule.DateField)+" of this record is "+event.Schedule.When+".")
		} else {
			return []string{"It would not run for this record now: " + name(event.Schedule.DateField) + " of this record is not " + event.Schedule.When + "."}
		}
	case event.OnCreate:
		lines = append(lines, "It runs when a record is created, so it would run for a new record like this one.")
	case event.WhenEquals != "":
		lines = append(lines, fmt.Sprintf("It runs when %s becomes %s; this record's %s is %s now, so it would run when that happens.", name(event.On), event.WhenEquals, name(event.On), shown(record.Values[event.On])))
	default:
		lines = append(lines, fmt.Sprintf("It runs when %s changes; this record's %s is %s now.", name(event.On), name(event.On), shown(record.Values[event.On])))
	}
	switch event.Then.Name {
	case domain.ServiceSendNotification:
		if event.Then.Notify == nil {
			break
		}
		ids := notificationRecipients(ctx, store, machine, record, *event.Then.Notify)
		if len(ids) == 0 {
			lines = append(lines, "It would tell no one: "+noRecipientWords(*event.Then.Notify)+".")
			break
		}
		var who []string
		for _, id := range ids {
			if u, err := store.GetRecord(ctx, domain.UserMachineID, id); err == nil {
				who = append(who, shown(u.Values["fld_name"]))
			}
		}
		lines = append(lines, fmt.Sprintf("It would tell %d: %s. Each gets it in the app, and by email where their own settings allow it. Nothing was sent.", len(ids), strings.Join(who, ", ")))
		lines = append(lines, "The message: "+renderEventSummary(event, machine, nil, record.Values))
	case domain.ServiceLogActivity:
		lines = append(lines, "It would write an activity entry: "+renderEventSummary(event, machine, nil, record.Values))
	case domain.ServiceSetField:
		if event.Then.Set != nil {
			lines = append(lines, fmt.Sprintf("It would set %s to %s on this record.", name(event.Then.Set.TargetField), event.Then.Set.Value))
		}
	case domain.ServiceRollupParentStatus:
		lines = append(lines, "It would look at the other records under the same parent and move the parent's status if they all agree.")
	default:
		lines = append(lines, "It would run its "+event.Then.Name+" step.")
	}
	return lines
}

func shown(v any) string {
	if s := displayString(v); s != "" {
		return s
	}
	return "empty"
}
