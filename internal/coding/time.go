package coding

import (
	"fmt"
	"time"

	"clara-code/internal/localtime"
)

const timeInstructions = "Interpret today, yesterday and tomorrow in the current host's local time zone. Use YYYY-MM-DD for dates and YYYY-MM-DD HH:mm:ss for full date-times shown to the user. Label other time zones explicitly. Keep timestamps in data and tool results as RFC3339 with Z or an explicit offset. Preserve the original meaning and time zones of historical timestamps; IDs are opaque identifiers, not clocks."

func (h Host) currentTime() time.Time {
	if h.now != nil {
		return h.now().Local()
	}
	return time.Now().Local()
}

func timeContext(at time.Time) string {
	at = at.Local()
	return fmt.Sprintf("\n## Current local time\nCurrent date: %s\nLocal date-time: %s\nRFC3339 timestamp: %s\nTime zone: %s\nThis clock snapshot applies to the current model step and supersedes earlier current-time statements. Do not retain it as the current clock in a handoff summary.\n",
		at.Format(time.DateOnly), localtime.DateTime(at), at.Format(time.RFC3339Nano), localtime.Zone(at))
}
