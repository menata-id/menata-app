package metadata

import (
	"fmt"

	"menata.app/internal/domain"
)

// validateBoardAddColumns is the half of a board's `add_column:` that needs the Application: its `to:` names a navigation item of an
// Application that installs the board's Machine, so the tile leads somewhere the viewer's own Application declares and a rename of that
// route moves the tile with it. A Machine no Application claims (a shared one) has no navigation to name and declares none.
func validateBoardAddColumns(applications []domain.Application, byID map[string]*domain.Machine) []string {
	var issues []string
	for _, app := range applications {
		for _, id := range app.Machines {
			m := byID[id]
			if m == nil {
				continue
			}
			for _, v := range m.Views {
				if v.AddColumn == nil {
					continue
				}
				if _, ok := app.NavigationItemByID(v.AddColumn.To); !ok {
					issues = append(issues, fmt.Sprintf("application %q: machine %q view %q: add_column.to %q is not a navigation item of this application", app.ID, m.ID, v.ID, v.AddColumn.To))
				}
			}
		}
	}
	return issues
}
