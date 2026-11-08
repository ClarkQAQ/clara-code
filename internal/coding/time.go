package coding

import (
	"time"
)

func (h Host) currentTime() time.Time {
	if h.now != nil {
		return h.now().Local()
	}
	return time.Now().Local()
}
