package controllers

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/go-git/go-billy/v5"
	gitv5 "github.com/go-git/go-git/v5"
	"github.com/pkg/errors"
)

func copy(data []byte, path string, fs billy.Filesystem, work *gitv5.Worktree) error {
	dst, err := openOrCreate(path, fs)
	if err != nil {
		return errors.Wrap(err, "failed to open")
	}
	src := bytes.NewBuffer(data)
	if _, err := io.Copy(dst, src); err != nil {
		return errors.Wrap(err, "failed to copy")
	}
	if err := dst.Close(); err != nil {
		return errors.Wrap(err, "failed to close")
	}
	_, err = work.Add(path)
	return errors.Wrap(err, "failed to add to git")
}

func deleteFile(path string, work *gitv5.Worktree, repoRoot string) error {
	fullPath := filepath.Join(repoRoot, path)
	err := os.Remove(fullPath)
	if err != nil {
		return errors.Wrap(err, "failed to delete file")
	}
	_, err = work.Add(path)
	if err != nil {
		return errors.Wrap(err, "failed to add to git")
	}
	return nil
}

func openOrCreate(path string, fs billy.Filesystem) (billy.File, error) {
	return fs.Create(path)
}

func findElement(list []string, element string) int {
	for i := range list {
		if list[i] == element {
			return i
		}
	}
	return -1
}

func removeElement(list []string, indext int) []string {
	return append(list[:indext], list[indext+1:]...)
}

func TabToSpace(input string) string {
	var result []string

	for _, i := range input {
		switch {
		// all these considered as space, including tab \t
		// '\t', '\n', '\v', '\f', '\r',' ', 0x85, 0xA0
		case unicode.IsSpace(i):
			result = append(result, " ") // replace tab with space
		case !unicode.IsSpace(i):
			result = append(result, string(i))
		}
	}
	return strings.Join(result, "")
}
