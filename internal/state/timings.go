package state

import "time"

const (
	PeerFailureDedupWindow = 4 * time.Second // concurrent probe failure dedup window
)
