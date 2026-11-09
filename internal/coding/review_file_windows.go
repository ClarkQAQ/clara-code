package coding

import (
	"errors"
	"fmt"
	"os"
)

func openReviewFile(root *os.Root, name string) (*os.File, error) {
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err == nil {
		var current os.FileInfo
		current, err = root.Lstat(name)
		if err == nil && (!current.Mode().IsRegular() || !os.SameFile(opened, current)) {
			err = fmt.Errorf("worktree file %q changed during review", name)
		}
	}
	if err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return file, nil
}
