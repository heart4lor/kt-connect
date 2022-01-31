package connect

import (
	"context"
	"github.com/alibaba/kt-connect/pkg/kt"
	"github.com/alibaba/kt-connect/pkg/kt/options"
	"github.com/alibaba/kt-connect/pkg/kt/sshuttle"
	"github.com/alibaba/kt-connect/pkg/kt/tunnel"
	"github.com/alibaba/kt-connect/pkg/kt/util"
	"github.com/rs/zerolog/log"
)

func BySshuttle(cli kt.CliInterface, options *options.DaemonOptions) error {
	checkSshuttleInstalled()

	podIP, podName, credential, err := getOrCreateShadow(cli.Kubernetes(), options)
	if err != nil {
		return err
	}

	cidrs, err := cli.Kubernetes().ClusterCidrs(context.TODO(), options.Namespace, options.ConnectOptions)
	if err != nil {
		return err
	}

	stop, rootCtx, err := tunnel.ForwardSSHTunnelToLocal(options, podName, options.ConnectOptions.SSHPort)
	if err != nil {
		return err
	}

	if err = startVPNConnection(rootCtx, options.ConnectOptions, &sshuttle.SSHVPNRequest{
		RemoteSSHHost:          credential.RemoteHost,
		RemoteSSHPKPath:        credential.PrivateKeyPath,
		RemoteDNSServerAddress: podIP,
		CustomCIDR:             cidrs,
		Stop:                   stop,
		Debug:                  options.Debug,
	}); err != nil {
		return err
	}

	return setupDns(cli, options, podIP)
}

func checkSshuttleInstalled() {
	if !util.CanRun(sshuttle.Ins().Version()) {
		_, _, err := util.RunAndWait(sshuttle.Ins().Install())
		if err != nil {
			log.Error().Err(err).Msgf("Failed find or install sshuttle")
		}
	}
}

func startVPNConnection(rootCtx context.Context, opt *options.ConnectOptions, req *sshuttle.SSHVPNRequest) (err error) {
	err = util.BackgroundRun(&util.CMDContext{
		Ctx:  rootCtx,
		Cmd:  sshuttle.Ins().Connect(opt, req),
		Name: "vpn(sshuttle)",
		Stop: req.Stop,
	})
	return err
}
