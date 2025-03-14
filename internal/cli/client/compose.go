package client

import "github.com/compose-spec/compose-go/v2/types"

type ComposeDeployment struct {
	project *types.Project
}

func NewComposeDeployment(project *types.Project) *ComposeDeployment {
	return &ComposeDeployment{project: project}
}

// TODO: implement Plan method to generate a deployment plan that consists of a sequence of operations with
//  service Deployments.
