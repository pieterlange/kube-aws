package root

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/coreos/kube-aws/cfnstack"
	controlplane "github.com/coreos/kube-aws/core/controlplane/cluster"
	controlplane_cfg "github.com/coreos/kube-aws/core/controlplane/config"
	nodepool "github.com/coreos/kube-aws/core/nodepool/cluster"
	nodepool_cfg "github.com/coreos/kube-aws/core/nodepool/config"
	"github.com/coreos/kube-aws/core/root/config"
	"github.com/coreos/kube-aws/core/root/defaults"
	"github.com/coreos/kube-aws/filereader/jsontemplate"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

const (
	LOCAL_ROOT_STACK_TEMPLATE_PATH = defaults.RootStackTemplateTmplFile
	REMOTE_STACK_TEMPLATE_FILENAME = "stack.json"
)

type Info struct {
	Name           string
	ControllerHost string
}

func (i *Info) String() string {
	return fmt.Sprintf("Name=%s, ControllerHost=%s", i.Name, i.ControllerHost)
}

func (c clusterImpl) Export() error {
	assets, err := c.assets()

	if err != nil {
		return err
	}

	for _, asset := range assets.AsMap() {
		path := filepath.Join("exported", "stacks", asset.Path)
		fmt.Printf("Exporting %s\n", path)
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("failed to create directory \"%s\": %v", dir, err)
		}
		if err := ioutil.WriteFile(path, []byte(asset.Content), 0600); err != nil {
			return fmt.Errorf("Error writing %s : %v", path, err)
		}
		if strings.HasSuffix(path, "stack.json") && c.controlPlane.KMSKeyARN == "" {
			fmt.Printf("BEWARE: %s contains your TLS secrets!\n", path)
		}
	}
	return nil
}

func (c clusterImpl) Info() (*Info, error) {
	return &Info{}, nil
}

type Cluster interface {
	Create() error
	Export() error
	Info() (*Info, error)
	Update() (string, error)
	ValidateStack() (string, error)
	ValidateTemplates() error
	ValidateUserData() error
}

func ClusterFromFile(configPath string, opts Options, awsDebug bool) (Cluster, error) {
	cfg, err := config.ConfigFromFile(configPath)
	if err != nil {
		return nil, err
	}
	return ClusterFromConfig(cfg, opts, awsDebug)
}

func normalizeS3URI(s3URI string) string {
	return strings.TrimSuffix(s3URI, "/")
}

func appendSuffixToS3URI(s3URI string, suffix string) string {
	return strings.Join([]string{normalizeS3URI(s3URI), suffix}, "/")
}

func ClusterFromConfig(cfg *config.Config, opts Options, awsDebug bool) (Cluster, error) {
	cpOpts := controlplane_cfg.StackTemplateOptions{
		TLSAssetsDir:          opts.TLSAssetsDir,
		ControllerTmplFile:    opts.ControllerTmplFile,
		EtcdTmplFile:          opts.EtcdTmplFile,
		StackTemplateTmplFile: opts.ControlPlaneStackTemplateTmplFile,
		PrettyPrint:           opts.PrettyPrint,
		S3URI:                 opts.S3URI,
	}
	cp, err := controlplane.NewCluster(cfg.Cluster, cpOpts, awsDebug)
	if err != nil {
		return nil, err
	}
	nodePools := []*nodepool.Cluster{}
	for i, c := range cfg.NodePools {
		npOpts := nodepool_cfg.StackTemplateOptions{
			TLSAssetsDir:          opts.TLSAssetsDir,
			WorkerTmplFile:        opts.WorkerTmplFile,
			StackTemplateTmplFile: opts.NodePoolStackTemplateTmplFile,
			PrettyPrint:           opts.PrettyPrint,
			S3URI:                 opts.S3URI,
		}
		np, err := nodepool.NewCluster(c, npOpts, awsDebug)
		if err != nil {
			return nil, fmt.Errorf("failed to load node pool #%d: %v", i, err)
		}
		nodePools = append(nodePools, np)
	}
	awsConfig := aws.NewConfig().
		WithRegion(cfg.Region).
		WithCredentialsChainVerboseErrors(true)

	if awsDebug {
		awsConfig = awsConfig.WithLogLevel(aws.LogDebug)
	}

	session, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to establish aws session: %v", err)
	}
	return clusterImpl{
		opts:         opts,
		controlPlane: cp,
		nodePools:    nodePools,
		session:      session,
	}, nil
}

type clusterImpl struct {
	controlPlane *controlplane.Cluster
	nodePools    []*nodepool.Cluster
	opts         Options
	session      *session.Session
}

func (c clusterImpl) Create() error {
	cfSvc := cloudformation.New(c.session)

	stackTemplateURL, err := c.prepareTemplateWithAssets()
	if err != nil {
		return err
	}

	return c.stackProvisioner().CreateStackAtURLAndWait(cfSvc, stackTemplateURL)
}

func (c clusterImpl) prepareTemplateWithAssets() (string, error) {
	assets, err := c.assets()

	if err != nil {
		return "", err
	}

	s3Svc := s3.New(c.session)
	err = c.stackProvisioner().UploadAssets(s3Svc, assets)
	if err != nil {
		return "", err
	}

	url := assets.FindAssetByStackAndFileName(c.stackName(), REMOTE_STACK_TEMPLATE_FILENAME).URL

	return url, nil
}

func (c clusterImpl) assets() (cfnstack.Assets, error) {
	stackTemplate, err := c.renderTemplateAsString()
	if err != nil {
		return nil, fmt.Errorf("Error while rendering template : %v", err)
	}
	assets := cfnstack.NewAssetsBuilder(c.stackName(), c.opts.S3URI).Add(REMOTE_STACK_TEMPLATE_FILENAME, stackTemplate).Build()

	cpAssets, err := c.controlPlane.Assets()
	if err != nil {
		return nil, err
	}
	assets = assets.Merge(cpAssets)

	for _, np := range c.nodePools {
		a, err := np.Assets()
		if err != nil {
			return nil, err
		}
		assets = assets.Merge(a)
	}

	return assets, nil
}

func (c clusterImpl) templatePath() string {
	return c.opts.RootStackTemplateTmplFile
}

func (c clusterImpl) templateParams() TemplateParams {
	params := newTemplateParams(c)
	return params
}

func (c clusterImpl) renderTemplateAsString() (string, error) {
	template, err := jsontemplate.GetString(c.templatePath(), c.templateParams(), c.opts.PrettyPrint)
	if err != nil {
		return "", err
	}
	return template, nil
}

func (c clusterImpl) stackProvisioner() *cfnstack.Provisioner {
	stackPolicyBody := `{
  "Statement" : [
    {
       "Effect" : "Allow",
       "Principal" : "*",
       "Action" : "Update:*",
       "Resource" : "*"
     }
  ]
}`
	return cfnstack.NewProvisioner(
		c.stackName(),
		c.tags(),
		c.opts.S3URI,
		stackPolicyBody,
		c.session)
}

func (c clusterImpl) stackName() string {
	return c.controlPlane.Cluster.ClusterName
}

func (c clusterImpl) tags() map[string]string {
	return c.controlPlane.Cluster.StackTags
}

func (c clusterImpl) Update() (string, error) {
	cfSvc := cloudformation.New(c.session)

	templateUrl, err := c.prepareTemplateWithAssets()
	if err != nil {
		return "", err
	}

	return c.stackProvisioner().UpdateStackAtURLAndWait(cfSvc, templateUrl)
}

func (c clusterImpl) ValidateUserData() error {
	if err := c.controlPlane.ValidateUserData(); err != nil {
		return fmt.Errorf("failed to validate control plane: %v", err)
	}
	for i, p := range c.nodePools {
		if err := p.ValidateUserData(); err != nil {
			return fmt.Errorf("failed to validate node pool #%d: %v", i, err)
		}
	}
	return nil
}

func (c clusterImpl) ValidateTemplates() error {
	_, err := c.renderTemplateAsString()
	if err != nil {
		return fmt.Errorf("failed to validate template: %v", err)
	}
	if _, err := c.controlPlane.RenderStackTemplateAsString(); err != nil {
		return fmt.Errorf("failed to validate control plane template: %v", err)
	}
	for i, p := range c.nodePools {
		if _, err := p.RenderStackTemplateAsString(); err != nil {
			return fmt.Errorf("failed to validate node pool #%d template: %v", i, err)
		}
	}
	return nil
}

func (c clusterImpl) ValidateStack() (string, error) {
	reports := []string{}

	cpReport, err := c.controlPlane.ValidateStack()
	if err != nil {
		return "", fmt.Errorf("failed to validate control plane: %v", err)
	}
	reports = append(reports, cpReport)

	for i, p := range c.nodePools {
		npReport, err := p.ValidateStack()
		if err != nil {
			return "", fmt.Errorf("failed to validate node pool #%d: %v", i, err)
		}
		reports = append(reports, npReport)
	}

	stackTemplate, err := c.renderTemplateAsString()
	if err != nil {
		return "", fmt.Errorf("failed to validate template : %v", err)
	}

	r, err := c.stackProvisioner().Validate(stackTemplate)
	if err != nil {
		return "", err
	}

	reports = append(reports, r)

	return strings.Join(reports, "\n"), nil
}
