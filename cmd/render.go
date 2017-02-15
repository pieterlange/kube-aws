package cmd

import (
	"fmt"
	"os"

	"github.com/coreos/kube-aws/core/root"
	"github.com/coreos/kube-aws/core/root/render"
	"github.com/spf13/cobra"
)

var (
	cmdRender = &cobra.Command{
		Use:          "render",
		Short:        "Render deployment artifacts",
		Long:         ``,
		RunE:         runCmdRender,
		SilenceUsage: true,
	}

	cmdRenderCredentials = &cobra.Command{
		Use:          "credentials",
		Short:        "Render TLS credentials",
		Long:         ``,
		RunE:         runCmdRenderCredentials,
		SilenceUsage: true,
	}

	renderCredentialsOpts = render.CredentialsOptions{}

	cmdRenderStack = &cobra.Command{
		Use:          "stack",
		Short:        "Render CloudFormation stack template and coreos-cloudinit userdata",
		Long:         ``,
		RunE:         runCmdRenderStack,
		SilenceUsage: true,
	}
)

func init() {
	RootCmd.AddCommand(cmdRender)

	cmdRender.AddCommand(cmdRenderCredentials)
	cmdRenderCredentials.Flags().BoolVar(&renderCredentialsOpts.GenerateCA, "generate-ca", false, "if generating credentials, generate root CA key and cert. NOT RECOMMENDED FOR PRODUCTION USE- use '-ca-key-path' and '-ca-cert-path' options to provide your own certificate authority assets")
	cmdRenderCredentials.Flags().StringVar(&renderCredentialsOpts.CaKeyPath, "ca-key-path", "./credentials/ca-key.pem", "path to pem-encoded CA RSA key")
	cmdRenderCredentials.Flags().StringVar(&renderCredentialsOpts.CaCertPath, "ca-cert-path", "./credentials/ca.pem", "path to pem-encoded CA x509 certificate")

	cmdRender.AddCommand(cmdRenderStack)
}
func runCmdRender(cmd *cobra.Command, args []string) error {
	fmt.Printf("WARNING: 'kube-aws render' is deprecated. See 'kube-aws render --help' for usage\n")
	if len(args) != 0 {
		return fmt.Errorf("render takes no arguments\n")
	}

	if _, err := os.Stat(renderCredentialsOpts.CaKeyPath); os.IsNotExist(err) {
		renderCredentialsOpts.GenerateCA = true
	}
	if err := runCmdRenderCredentials(cmdRenderCredentials, args); err != nil {
		return err
	}

	if err := runCmdRenderStack(cmdRenderCredentials, args); err != nil {
		return err
	}

	return nil
}
func runCmdRenderStack(cmd *cobra.Command, args []string) error {
	// Read the config from file.
	cluster, err := root.StackAssetsRendererFromFile(configPath)
	if err != nil {
		return fmt.Errorf("Failed to read cluster config: %v", err)
	}

	if err := cluster.RenderFiles(); err != nil {
		return err
	}

	successMsg :=
		`Success! Stack rendered to ./stack-templates.

Next steps:
1. (Optional) Validate your changes to %s with "kube-aws validate"
2. (Optional) Further customize the cluster by modifying templates in ./stack-templates or cloud-configs in ./userdata.
3. Start the cluster with "kube-aws up".
`

	fmt.Printf(successMsg, configPath)
	return nil
}

func runCmdRenderCredentials(cmd *cobra.Command, args []string) error {
	cluster, err := root.CredentialsRendererFromFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read cluster config: %v", err)
	}

	return cluster.RenderFiles(renderCredentialsOpts)
}
