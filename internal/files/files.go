package files

import "os"

func fileExistsQ(fname string) bool {
	info, err := os.Stat(fname)
	if os.IsNotExist(err) {
		return false
	}
	// if err is not nil and it's not a directory then it must be a file
	return err == nil && !info.IsDir()
}

// EnsureExistsAndOpen will ensure that a file exists in order to open it and
// return the file handle, optionally deleting the file if it already exists
// TODO: support an option for whether to append or not?
func EnsureExistsAndOpen(fname string, delete bool) (*os.File, error) {
	// if the file doesn't exist, create it
	fExists := fileExistsQ(fname)
	switch {
	case fExists && !delete:
		// open to append the file
		return os.OpenFile(fname, os.O_WRONLY|os.O_APPEND, 0644)
	case fExists && delete:
		// delete the file and then fallthrough to create the file
		err := os.Remove(fname)
		if err != nil {
			return nil, err
		}
		fallthrough
	default:
		// file doesn't exist or err'd stat'ing file, in which case create will
		// also fail, but then the user can inspect the Create error for details
		return os.Create(fname)
	}
}
