package options

import (
	"github.com/alibaba/kt-connect/pkg/kt/util"
)

// ConnectOptions ...
type ConnectOptions struct {
	Global           bool
	DisablePodIp     bool
	DisableTunDevice bool
	DisableTunRoute  bool
	SocksPort        int
	DnsPort          int
	DnsCacheTtl      int
	IncludeIps       string
	ExcludeIps       string
	Mode             string
	DnsMode          string
	SharedShadow     bool
	ClusterDomain    string
	SkipCleanup      bool
}

// ExchangeOptions ...
type ExchangeOptions struct {
	Mode            string
	Expose          string
	RecoverWaitTime int
}

// MeshOptions ...
type MeshOptions struct {
	Mode        string
	Expose      string
	VersionMark string
	RouterImage string
}

// RecoverOptions ...
type RecoverOptions struct {

}

// PreviewOptions ...
type PreviewOptions struct {
	External bool
	Expose   string
}

// CleanOptions ...
type CleanOptions struct {
	DryRun           bool
	ThresholdInMinus int64
	SweepLocalRoute  bool
}

// ConfigOptions ...
type ConfigOptions struct {

}

// GlobalOptions ...
type GlobalOptions struct {
	RunAsWorkerProcess  bool
	KubeConfig          string
	Namespace           string
	ServiceAccount      string
	Debug               bool
	Image               string
	ImagePullSecret     string
	NodeSelector        string
	WithLabels          string
	WithAnnotations     string
	PortForwardWaitTime int
	PodCreationWaitTime int
	UseShadowDeployment bool
	AlwaysUpdateShadow  bool
	SkipTimeDiff        bool
	KubeContext         string
	PodQuota            string
}

// DaemonOptions cli options
type DaemonOptions struct {
	Connect  *ConnectOptions
	Exchange *ExchangeOptions
	Mesh     *MeshOptions
	Preview  *PreviewOptions
	Recover  *RecoverOptions
	Clean    *CleanOptions
	Config   *ConfigOptions
	Global   *GlobalOptions
}

var opt *DaemonOptions

// Get fetch options instance
func Get() *DaemonOptions {
	if opt == nil {
		opt = &DaemonOptions{
			Global: &GlobalOptions {
				Namespace:  util.DefaultNamespace,
			},
			Connect:  &ConnectOptions{},
			Exchange: &ExchangeOptions{},
			Mesh:     &MeshOptions{},
			Preview:  &PreviewOptions{},
			Clean:    &CleanOptions{},
		}
	}
	return opt
}