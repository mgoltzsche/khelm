package helm

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

func writeFileAtomic(src io.Reader, destFile string) (err error) {
	tmp, err := ioutil.TempFile(filepath.Dir(destFile), ".tmp-"+filepath.Base(destFile)+"-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = io.Copy(tmp, src)
	e := tmp.Close()
	if err != nil {
		return err
	}
	if e != nil {
		return e
	}
	err = os.Rename(tmp.Name(), destFile)
	return err
}
