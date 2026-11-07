package localtime

import (
	"strings"
	"time"
)

func DateTime(at time.Time) string {
	return at.Local().Format(time.DateTime)
}

func Zone(at time.Time) string {
	return zone(at.Local())
}

func zone(at time.Time) string {
	offset := "UTC" + at.Format("-07:00")
	name := at.Location().String()
	if name == "UTC" {
		return "UTC (+00:00)"
	}
	if strings.Contains(name, "/") {
		return name + " (" + offset + ")"
	}
	return offset
}
