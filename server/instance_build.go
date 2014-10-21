package server

import "fmt"

var (
	errEmptySite         = fmt.Errorf("empty \"site\" param")
	errEmptyEnv          = fmt.Errorf("empty \"env\" param")
	errEmptyQueue        = fmt.Errorf("empty \"queue\" param")
	errEmptyInstanceType = fmt.Errorf("empty \"instance_type\" param")
)

type instanceBuildsCollectionSingular struct {
	InstanceBuilds *instanceBuild `json:"instance_builds"`
}

type instanceBuildsCollection struct {
	InstanceBuilds []*instanceBuild `json:"instance_builds"`
}

type instanceBuild struct {
	Site         string `json:"site"`
	Env          string `json:"env"`
	AMI          string `json:"ami"`
	InstanceType string `json:"instance_type"`
	Count        int    `json:"count"`
	Queue        string `json:"queue"`
	HREF         string `json:"href,omitempty"`
	ID           string `json:"id,omitempty"`
}

func (b *instanceBuild) Validate() []error {
	errors := []error{}
	if b.Site == "" {
		errors = append(errors, errEmptySite)
	}
	if b.Env == "" {
		errors = append(errors, errEmptyEnv)
	}
	if b.Queue == "" {
		errors = append(errors, errEmptyQueue)
	}
	if b.InstanceType == "" {
		errors = append(errors, errEmptyInstanceType)
	}

	return errors
}

func (b *instanceBuild) UpdateFromDetails(d *instanceBuildDetails) {
	if d.ID != "" {
		b.ID = d.ID
	}

	return
}
