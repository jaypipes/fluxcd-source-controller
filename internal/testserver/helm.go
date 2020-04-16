package testserver

import (
	"io/ioutil"
	"path/filepath"

	"helm.sh/helm/v3/pkg/repo"
	"sigs.k8s.io/yaml"
)

type Helm struct {
	*HTTP
}

func (s *Helm) GenerateIndex() error {
	index, err := repo.IndexDirectory(s.HTTP.docroot, s.HTTP.URL())
	if err != nil {
		return err
	}
	d, err := yaml.Marshal(index)
	if err != nil {
		return err
	}
	f := filepath.Join(s.HTTP.docroot, "index.yaml")
	return ioutil.WriteFile(f, d, 0644)
}
