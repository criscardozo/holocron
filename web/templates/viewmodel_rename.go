package templates

// RenameFileRow is one file changing name inside a folder.
type RenameFileRow struct {
	From string
	To   string
}

// RenamePlanRow is one folder's worth of proposed change.
type RenamePlanRow struct {
	// Key is the absolute path, and what the apply form sends back.
	Key   string
	Label string
	From  string
	To    string
	Files []RenameFileRow
	// Skipped is what would be left alone inside this folder, with the reason.
	Skipped []string
}

// FolderChanges reports whether the folder itself is being renamed, as opposed
// to only the files inside it.
func (r RenamePlanRow) FolderChanges() bool { return r.From != r.To }

// RenameBlockedRow is a folder Holocron will not touch, and why.
type RenameBlockedRow struct {
	Label  string
	Folder string
	Reason string
}

// RenamePageView drives the bulk rename screen.
type RenamePageView struct {
	HasMediaFolders bool
	// Running is true while a preview or an apply is in flight.
	Running bool
	// Previewed is false before the first preview, which is different from a
	// preview that found nothing: one says "ask me", the other "nothing to do".
	Previewed  bool
	Plans      []RenamePlanRow
	Blocked    []RenameBlockedRow
	TotalFiles int
	Notice     string
	NoticeErr  bool
}
