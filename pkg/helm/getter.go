package helm

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/hashicorp/go-getter"
	"github.com/pkg/errors"
	helmgetter "k8s.io/helm/pkg/getter"
	"k8s.io/helm/pkg/helm/environment"
)

func getters(settings environment.EnvSettings) helmgetter.Providers {
	providers := helmgetter.All(settings)
	providers = append(providers, helmgetter.Provider{
		Schemes: []string{"git", "file"},
		New:     newGoGetterConstructor,
	})
	return providers
}

func newGoGetterConstructor(URL, CertFile, KeyFile, CAFile string) (g helmgetter.Getter, err error) {
	dir, err := ioutil.TempDir("", "khelmgetter-")
	if err != nil {
		return
	}
	os.RemoveAll(dir)
	fmt.Println("## construct getter: ", URL)
	err = getter.Get(dir, URL)
	// TODO: remove dir after helm rendering is done
	/*u, err := url.Parse(URL)
	if err != nil {
		return
	}
	dir := u.Path
	if u.RawPath != "" {
		dir = u.RawPath
	}*/
	return &goGetter{dir}, errors.Wrap(err, "cannot resolve getter")
}

type goGetter struct {
	dir string
}

func (g *goGetter) Get(url string) (bu *bytes.Buffer, err error) {
	fmt.Println("## get", url)
	b, err := ioutil.ReadFile(filepath.Join(g.dir, url))
	if err != nil {
		return
	}
	bu = &bytes.Buffer{}
	_, err = bu.Write(b)
	return
}

/*
func newGoGetter() helmgetter.Getter {
	return &
}*/
