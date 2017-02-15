package cluster

import (
	controlplane "github.com/coreos/kube-aws/core/controlplane/config"
	"github.com/coreos/kube-aws/core/nodepool/config"
)

func ClusterRefFromBytes(bytes []byte, main *controlplane.Config, awsDebug bool) (*ClusterRef, error) {
	provided, err := config.ClusterFromBytes(bytes, main)
	if err != nil {
		return nil, err
	}
	c := NewClusterRef(provided, awsDebug)
	return c, nil
}
