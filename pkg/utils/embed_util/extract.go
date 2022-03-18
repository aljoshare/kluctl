package embed_util

import (
	"github.com/codablock/kluctl/pkg/utils"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func ExtractTarToTmp(r io.Reader, fileListR io.Reader, targetPath string) error {
	fileList, err := ioutil.ReadAll(fileListR)
	if err != nil {
		return err
	}

	needsExtract, expectedTarGzHash, err := checkExtractNeeded(targetPath, string(fileList))
	if err != nil {
		return err
	}
	if !needsExtract {
		return nil
	}

	tmpTargetPath := targetPath + ".tmp"
	err = os.MkdirAll(tmpTargetPath , 0o700)
	if err != nil {
		return err
	}

	err = utils.ExtractTarGzStream(r, tmpTargetPath)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(filepath.Join(tmpTargetPath, ".tar-gz-hash"), []byte(expectedTarGzHash), 0o600)
	if err != nil {
		return err
	}

	_ = os.RemoveAll(targetPath)
	err = os.Rename(tmpTargetPath, targetPath)
	if err != nil {
		return err
	}

	return nil
}

func checkExtractNeeded(targetPath string, fileListStr string) (bool, string, error) {
	fileList := strings.Split(fileListStr, "\n")
	expectedHash := fileList[0]
	fileList = fileList[1:]

	if !utils.Exists(targetPath) {
		return true, expectedHash, nil
	}

	existingHash, err := ioutil.ReadFile(filepath.Join(targetPath, ".tar-gz-hash"))
	if err != nil {
		return true, expectedHash, nil
	}

	if strings.TrimSpace(expectedHash) != strings.TrimSpace(string(existingHash)) {
		return true, expectedHash, nil
	}

	tarFilesMap := make(map[string]int64)
	for _, l := range fileList {
		s := strings.Split(l, ":")
		fname := strings.TrimSpace(s[0])
		size, err := strconv.ParseInt(strings.TrimSpace(s[1]), 10, 64)
		if err != nil {
			return false, expectedHash, err
		}
		tarFilesMap[fname] = size
	}

	existingFiles := make(map[string]int64)
	err = filepath.Walk(targetPath, func(path string, info fs.FileInfo, err error) error {
		if !info.Mode().IsRegular() && !info.IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(targetPath, path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			existingFiles[relPath] = 0
		} else {
			existingFiles[relPath] = info.Size()
		}
		return nil
	})
	if err != nil {
		return false, "", err
	}

	for fname, size := range tarFilesMap {
		if s, ok := existingFiles[fname]; !ok || s != size {
			return true, expectedHash, nil
		}
	}
	return false, expectedHash, nil
}