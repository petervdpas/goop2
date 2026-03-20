package state

import "time"

const (
	PeerFailureDedupWindow = 2 * time.Second // concurrent probe failure dedup window
)
