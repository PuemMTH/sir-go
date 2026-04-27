package main

import (
	"os"

	"gopkg.in/yaml.v3"
)

type composeFile struct {
	Name     string                 `yaml:"name"`
	Services map[string]interface{} `yaml:"services"`
}

func parseCompose(fp string) (string, []string, error) {
	data, err := os.ReadFile(fp)
	if err != nil {
		return "", nil, err
	}
	var cf composeFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return "", nil, err
	}
	var svcs []string
	for name := range cf.Services {
		svcs = append(svcs, name)
	}
	return cf.Name, svcs, nil
}
