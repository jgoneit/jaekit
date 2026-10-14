package history

import "golang.org/x/sys/unix"

// Pin directories without requiring permission to list their entries.
const historyDirectoryAccess = unix.O_SEARCH
