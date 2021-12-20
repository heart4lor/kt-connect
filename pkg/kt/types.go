package kt

import (
	"github.com/alibaba/kt-connect/pkg/kt/cluster"
	"github.com/alibaba/kt-connect/pkg/kt/exec"
	"github.com/alibaba/kt-connect/pkg/kt/options"
	"github.com/alibaba/kt-connect/pkg/kt/util"
)

// CliInterface ...
type CliInterface interface {
	Kubernetes() (cluster.KubernetesInterface, error)
	Exec() exec.CliInterface
}

// Cli ...
type Cli struct {
	Options *options.DaemonOptions
}

// Kubernetes ...
func (c *Cli) Kubernetes() (cluster.KubernetesInterface, error) {
	return cluster.CreateFromClientSet(c.Options.RuntimeOptions.Clientset)
}

// Exec ...
func (c *Cli) Exec() exec.CliInterface {
	return &exec.Cli{
		KubeOptions: c.Options.KubeOptions,
		TunName:     c.Options.ConnectOptions.TunName,
		SourceIP:    c.Options.ConnectOptions.SourceIP,
		DestIP:      c.Options.ConnectOptions.DestIP,
		MaskLen:     util.ExtractNetMaskFromCidr(c.Options.ConnectOptions.TunCidr),
	}
}
