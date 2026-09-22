package capture

// Showable reports whether the panel would draw this event.
//
// There is one definition because there used to be two. The panel hid some
// recorded events, while the decision to open a popup counted every event on
// disk — so a turn whose only writes were a shell command touching files
// outside the project opened a popup with nothing new in it, and a session
// whose every event was like that opened an empty one.
//
// Two kinds are not worth reading. Anything hidden is machinery rather than
// work. And a shell command's writes outside the project are things the file
// walk swept up rather than things anyone asked for — another program's scratch
// files, a temp repository. An edit made deliberately with Edit or Write is a
// different matter: outside the project or not, someone meant it.
func Showable(root string, e Event) bool {
	if e.Path == "" {
		return true // nothing to judge it by
	}
	if HiddenPath(root, e.Path) {
		return false
	}
	if e.Tool == "Bash" && !Inside(root, e.Path) {
		return false
	}
	return true
}

// CountShowable is how many of these the panel would draw.
func CountShowable(root string, events []Event) int {
	n := 0
	for _, e := range events {
		if Showable(root, e) {
			n++
		}
	}
	return n
}

// LastShowable is the highest sequence the panel would draw, or zero if it
// would draw nothing.
//
// This is what a popup should be opened for: a turn that produced only hidden
// or strayed writes leaves the marker where it was, and opens nothing.
func LastShowable(root string, events []Event) int {
	last := 0
	for _, e := range events {
		if Showable(root, e) && e.Seq > last {
			last = e.Seq
		}
	}
	return last
}
