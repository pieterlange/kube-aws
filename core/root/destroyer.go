package root

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/coreos/kube-aws/cfnstack"
	"github.com/coreos/kube-aws/core/root/config"
)

type DestroyOptions struct {
	AwsDebug bool
}

type ClusterDestroyer interface {
	Destroy() error
}

type clusterDestroyerImpl struct {
	underlying *cfnstack.Destroyer
}

func ClusterDestroyerFromFile(configPath string, opts DestroyOptions) (ClusterDestroyer, error) {
	cfg, err := config.ConfigFromFile(configPath)
	if err != nil {
		return nil, err
	}

	region := cfg.Region
	stackName := cfg.RootStackName()

	awsConfig := aws.NewConfig().
		WithRegion(region).
		WithCredentialsChainVerboseErrors(true)

	if opts.AwsDebug {
		awsConfig = awsConfig.WithLogLevel(aws.LogDebug)
	}

	session, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to establish aws session: %v", err)
	}

	cfnDestroyer := cfnstack.NewDestroyer(stackName, session)
	return clusterDestroyerImpl{
		underlying: cfnDestroyer,
	}, nil
}

func (d clusterDestroyerImpl) Destroy() error {
	return d.underlying.Destroy()
}
