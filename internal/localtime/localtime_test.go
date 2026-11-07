package localtime

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestLocalTimeRespectsTZ(t *testing.T) {
	type fixture struct {
		TZ, At, DateTime, Zone string
	}
	if raw := os.Getenv("CLARA_TEST_LOCAL_TIME"); raw != "" {
		var test fixture
		if err := json.Unmarshal([]byte(raw), &test); err != nil {
			t.Fatal(err)
		}
		at, err := time.Parse(time.RFC3339, test.At)
		if err != nil {
			t.Fatal(err)
		}
		if got := DateTime(at); got != test.DateTime {
			t.Fatalf("local date-time %q, want %q", got, test.DateTime)
		}
		if got := Zone(at); got != test.Zone {
			t.Fatalf("local zone %q, want %q", got, test.Zone)
		}
		return
	}
	for _, test := range []fixture{
		{"Asia/Shanghai", "2026-11-07T17:02:03Z", "2026-11-08 01:02:03", "Asia/Shanghai (UTC+08:00)"},
		{"America/New_York", "2026-01-01T02:03:04Z", "2025-12-31 21:03:04", "America/New_York (UTC-05:00)"},
		{"America/New_York", "2026-07-01T02:03:04Z", "2026-06-30 22:03:04", "America/New_York (UTC-04:00)"},
		{"Asia/Kathmandu", "2026-11-07T19:02:03Z", "2026-11-08 00:47:03", "Asia/Kathmandu (UTC+05:45)"},
		{"UTC", "2026-11-07T23:59:59Z", "2026-11-07 23:59:59", "UTC (+00:00)"},
	} {
		t.Run(test.TZ+test.At, func(t *testing.T) {
			data, _ := json.Marshal(test)
			child := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestLocalTimeRespectsTZ$")
			child.Env = append(os.Environ(), "TZ="+test.TZ, "CLARA_TEST_LOCAL_TIME="+string(data))
			if output, err := child.CombinedOutput(); err != nil {
				t.Fatalf("TZ=%s: %v\n%s", test.TZ, err, output)
			}
		})
	}
}

func TestZoneDoesNotGuessFromAbbreviations(t *testing.T) {
	for _, test := range []struct {
		name, want string
		offset     int
	}{
		{"CST", "UTC+08:00", 8 * 3600},
		{"CST", "UTC-06:00", -6 * 3600},
		{"Local", "UTC+05:30", 5*3600 + 30*60},
		{"", "UTC+00:00", 0},
	} {
		at := time.Date(2026, 11, 8, 1, 2, 3, 0, time.FixedZone(test.name, test.offset))
		if got := zone(at); got != test.want {
			t.Fatalf("zone %q = %q, want %q", test.name, got, test.want)
		}
	}
}
