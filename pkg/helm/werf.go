package helm

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/romana/rlog"

	"github.com/flant/shell-operator/pkg/executor"

	"github.com/flant/addon-operator/pkg/app"
)

const WerfPath = "werf"

// WerfOptions
// FIXME is this needed?
type WerfOptions struct {
	HelmReleaseStorageNamespace string
	Namespace string
}

type WerfClient interface {
	DeployChart(releaseName string, chart string, valuesPaths []string, setValues []string, namespace string) error
}

type werfClient struct {
	Options WerfOptions
}

// werfClient implements WerfClient
var _ WerfClient = &werfClient{}

func NewWerfClient(opts WerfOptions) WerfClient {
	return &werfClient{
		Options: opts,
	}
}

func (w *werfClient) DeployChart(releaseName string, chart string, valuesPaths []string, setValues []string, namespace string) error {
	args := make([]string, 0)
	args = append(args, "helm")
	args = append(args, "deploy-chart")

	ns := namespace
	if app.WerfTillerNamespace != "" {
		ns = app.WerfTillerNamespace
	}
	args = append(args, "--namespace")
	args = append(args, ns)
	args = append(args, "--helm-release-storage-namespace")
	args = append(args, ns)

	for _, valuesPath := range valuesPaths {
		args = append(args, "--values")
		args = append(args, valuesPath)
	}

	for _, setValue := range setValues {
		args = append(args, "--set")
		args = append(args, setValue)
	}

	args = append(args, chart)
	args = append(args, releaseName)

	rlog.Infof("Running werf helm deploy-chart for release '%s' with chart '%s' in namespace '%s' ...", releaseName, chart, ns)
	stdout, stderr, err := w.Run(args...)
	if err != nil {
		return fmt.Errorf("werf helm deploy-chart failed: %s:\n%s %s", err, stdout, stderr)
	}
	rlog.Infof("werf helm deploy-chart for release '%s' with chart '%s' in namespace '%s' was successful:\n%s\n%s", releaseName, chart, ns, stdout, stderr)

	return nil
}


func (w *werfClient) TillerNamespace() string {
	//return h.tillerNamespace
	return w.Options.Namespace
}

// Cmd starts Helm with specified arguments.
// Sets the TILLER_NAMESPACE environment variable before starting, because Addon-operator works with its own Tiller.
func (w *werfClient) Run(args ...string) (stdout string, stderr string, err error) {
	cmd := exec.Command(WerfPath, args...)
	cmd.Env = os.Environ()
	if app.WerfTillerNamespace != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("TILLER_NAMESPACE=%s", app.WerfTillerNamespace))
	}

	var stdoutBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	err = executor.Run(cmd, true)
	stdout = strings.TrimSpace(stdoutBuf.String())
	stderr = strings.TrimSpace(stderrBuf.String())

	return
}
