package command

import (
	"context"
	"fmt"
	"github.com/alibaba/kt-connect/pkg/common"
	"github.com/alibaba/kt-connect/pkg/kt/command/general"
	cluster2 "github.com/alibaba/kt-connect/pkg/kt/service/cluster"
	dns2 "github.com/alibaba/kt-connect/pkg/kt/service/dns"
	opt "github.com/alibaba/kt-connect/pkg/kt/options"
	"github.com/alibaba/kt-connect/pkg/kt/util"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	urfave "github.com/urfave/cli"
	"io/ioutil"
	coreV1 "k8s.io/api/core/v1"
	"os"
	"strconv"
	"strings"
	"time"
)

type ResourceToClean struct {
	PodsToDelete        []string
	ServicesToDelete    []string
	ConfigMapsToDelete  []string
	DeploymentsToScale  map[string]int32
	ServicesToRecover   []string
	ServicesToUnlock   []string
}

// NewCleanCommand return new connect command
func NewCleanCommand(action ActionInterface) urfave.Command {
	return urfave.Command{
		Name:  "clean",
		Usage: "delete unavailing shadow pods from kubernetes cluster",
		UsageText: "ktctl clean [command options]",
		Flags: general.CleanActionFlag(opt.Get()),
		Action: func(c *urfave.Context) error {
			if opt.Get().Debug {
				zerolog.SetGlobalLevel(zerolog.DebugLevel)
			}
			if err := general.CombineKubeOpts(); err != nil {
				return err
			}
			return action.Clean()
		},
	}
}

//Clean delete unavailing shadow pods
func (action *Action) Clean() error {
	action.cleanPidFiles()
	ctx := context.Background()

	pods, cfs, svcs, err := cluster2.GetKtResources(ctx, opt.Get().Namespace)
	if err != nil {
		return err
	}
	log.Debug().Msgf("Find %d kt pods", len(pods))
	resourceToClean := ResourceToClean{
		PodsToDelete: make([]string, 0),
		ServicesToDelete: make([]string, 0),
		ConfigMapsToDelete: make([]string, 0),
		DeploymentsToScale: make(map[string]int32),
		ServicesToRecover: make([]string, 0),
		ServicesToUnlock: make([]string, 0),
	}
	for _, pod := range pods {
		action.analysisExpiredPods(pod, opt.Get().CleanOptions.ThresholdInMinus, &resourceToClean)
	}
	for _, cf := range cfs {
		action.analysisExpiredConfigmaps(cf, opt.Get().CleanOptions.ThresholdInMinus, &resourceToClean)
	}
	for _, svc := range svcs {
		action.analysisExpiredServices(svc, opt.Get().CleanOptions.ThresholdInMinus, &resourceToClean)
	}
	svcList, err := cluster2.Ins().GetAllServiceInNamespace(ctx, opt.Get().Namespace)
	action.analysisLocked(svcList.Items, &resourceToClean)
	if isEmpty(resourceToClean) {
		log.Info().Msg("No unavailing kt resource found (^.^)YYa!!")
	} else {
		if opt.Get().CleanOptions.DryRun {
			action.printResourceToClean(resourceToClean)
		} else {
			action.cleanResource(ctx, resourceToClean, opt.Get().Namespace)
		}
	}

	if !opt.Get().CleanOptions.DryRun {
		log.Debug().Msg("Cleaning up unused local rsa keys ...")
		util.CleanRsaKeys()
		if util.GetDaemonRunning(common.ComponentConnect) < 0 {
			log.Debug().Msg("Cleaning up hosts file ...")
			dns2.DropHosts()
			log.Debug().Msg("Cleaning DNS configuration ...")
			dns2.Ins().RestoreNameServer()
		}
	}
	return nil
}

func (action *Action) cleanPidFiles() {
	files, _ := ioutil.ReadDir(common.KtHome)
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".pid") {
			component, pid := action.parseComponentAndPid(f.Name())
			if util.IsProcessExist(pid) {
				log.Debug().Msgf("Find kt %s instance with pid %d", component, pid)
			} else {
				log.Info().Msgf("Removing remnant pid file %s", f.Name())
				if err := os.Remove(fmt.Sprintf("%s/%s", common.KtHome, f.Name())); err != nil {
					log.Error().Err(err).Msgf("Delete pid file %s failed", f.Name())
				}
			}
		}
	}
}

func (action *Action) analysisExpiredPods(pod coreV1.Pod, cleanThresholdInMinus int64, resourceToClean *ResourceToClean) {
	lastHeartBeat := util.ParseTimestamp(pod.Annotations[common.KtLastHeartBeat])
	if lastHeartBeat > 0 && isExpired(lastHeartBeat, cleanThresholdInMinus) {
		log.Debug().Msgf(" * pod %s expired, lastHeartBeat: %d ", pod.Name, lastHeartBeat)
		if pod.DeletionTimestamp == nil {
			resourceToClean.PodsToDelete = append(resourceToClean.PodsToDelete, pod.Name)
		}
		log.Debug().Msgf("   role %s, config: %s", pod.Labels[common.KtRole], pod.Annotations[common.KtConfig])
		config := util.String2Map(pod.Annotations[common.KtConfig])
		if pod.Labels[common.KtRole] == common.RoleExchangeShadow {
			replica, _ := strconv.ParseInt(config["replicas"], 10, 32)
			app := config["app"]
			if replica > 0 && app != "" {
				resourceToClean.DeploymentsToScale[app] = int32(replica)
			}
		} else if pod.Labels[common.KtRole] == common.RoleRouter {
			if service, ok := config["service"]; ok {
				resourceToClean.ServicesToRecover = append(resourceToClean.ServicesToRecover, service)
			}
		}
	} else {
		log.Debug().Msgf("Pod %s does no have heart beat annotation", pod.Name)
	}
}

func (action *Action) analysisExpiredConfigmaps(cf coreV1.ConfigMap, cleanThresholdInMinus int64, resourceToClean *ResourceToClean) {
	lastHeartBeat := util.ParseTimestamp(cf.Annotations[common.KtLastHeartBeat])
	if lastHeartBeat > 0 && isExpired(lastHeartBeat, cleanThresholdInMinus) {
		resourceToClean.ConfigMapsToDelete = append(resourceToClean.ConfigMapsToDelete, cf.Name)
	}
}

func (action *Action) analysisExpiredServices(svc coreV1.Service, cleanThresholdInMinus int64, resourceToClean *ResourceToClean) {
	lastHeartBeat := util.ParseTimestamp(svc.Annotations[common.KtLastHeartBeat])
	if lastHeartBeat > 0 && isExpired(lastHeartBeat, cleanThresholdInMinus) {
		resourceToClean.ServicesToDelete = append(resourceToClean.ServicesToDelete, svc.Name)
	}
}

func (action *Action) analysisLocked(svcs []coreV1.Service, resourceToClean *ResourceToClean) {
	for _, svc := range svcs {
		if svc.Annotations == nil {
			continue
		}
		if lock, ok := svc.Annotations[common.KtLock]; ok && time.Now().Unix() - util.ParseTimestamp(lock) > general.LockTimeout {
			resourceToClean.ServicesToUnlock = append(resourceToClean.ServicesToUnlock, svc.Name)
		}
	}
}

func (action *Action) cleanResource(ctx context.Context, r ResourceToClean, namespace string) {
	log.Info().Msgf("Deleting %d unavailing kt pods", len(r.PodsToDelete))
	for _, name := range r.PodsToDelete {
		err := cluster2.Ins().RemovePod(ctx, name, namespace)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to delete pods %s", name)
		}
	}
	log.Info().Msgf("Deleting %d unavailing config maps", len(r.ConfigMapsToDelete))
	for _, name := range r.ConfigMapsToDelete {
		err := cluster2.Ins().RemoveConfigMap(ctx, name, namespace)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to delete config map %s", name)
		}
	}
	log.Info().Msgf("Recovering %d scaled deployments", len(r.DeploymentsToScale))
	for name, replica := range r.DeploymentsToScale {
		err := cluster2.Ins().ScaleTo(ctx, name, namespace, &replica)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to scale deployment %s to %d", name, replica)
		}
	}
	log.Info().Msgf("Deleting %d unavailing services", len(r.ServicesToDelete))
	for _, name := range r.ServicesToDelete {
		err := cluster2.Ins().RemoveService(ctx, name, namespace)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to delete service %s", name)
		}
	}
	log.Info().Msgf("Recovering %d meshed services", len(r.ServicesToRecover))
	for _, name := range r.ServicesToRecover {
		general.RecoverOriginalService(ctx, name, namespace)
	}
	log.Info().Msgf("Recovering %d locked services", len(r.ServicesToUnlock))
	for _, name := range r.ServicesToUnlock {
		if app, err := cluster2.Ins().GetService(ctx, name, namespace); err == nil {
			delete(app.Annotations, common.KtLock)
			_, err = cluster2.Ins().UpdateService(ctx, app)
			if err != nil {
				log.Error().Err(err).Msgf("Failed to lock service %s", name)
			}
		}
	}
	log.Info().Msg("Done")
}

func (action *Action) parseComponentAndPid(pidFileName string) (string, int) {
	startPos := strings.LastIndex(pidFileName, "-")
	endPos := strings.Index(pidFileName, ".")
	if startPos > 0 && endPos > startPos {
		component := pidFileName[0 : startPos]
		pid, err := strconv.Atoi(pidFileName[startPos+1 : endPos])
		if err != nil {
			return "", -1
		}
		return component, pid
	}
	return "", -1
}

func (action *Action) printResourceToClean(r ResourceToClean) {
	log.Info().Msgf("Find %d unavailing pods to delete:", len(r.PodsToDelete))
	for _, name := range r.PodsToDelete {
		log.Info().Msgf(" * %s", name)
	}
	log.Info().Msgf("Find %d unavailing config maps to delete:", len(r.ConfigMapsToDelete))
	for _, name := range r.ConfigMapsToDelete {
		log.Info().Msgf(" * %s", name)
	}
	log.Info().Msgf("Find %d exchanged deployments to recover:", len(r.DeploymentsToScale))
	for name, replica := range r.DeploymentsToScale {
		log.Info().Msgf(" * %s -> %d", name, replica)
	}
	log.Info().Msgf("Find %d unavailing service to delete:", len(r.ServicesToDelete))
	for _, name := range r.ServicesToDelete {
		log.Info().Msgf(" * %s", name)
	}
	log.Info().Msgf("Find %d meshed service to recover:", len(r.ServicesToRecover))
	for _, name := range r.ServicesToRecover {
		log.Info().Msgf(" * %s", name)
	}
	log.Info().Msgf("Find %d locked services to recover:", len(r.ServicesToUnlock))
	for _, name := range r.ServicesToUnlock {
		log.Info().Msgf(" * %s", name)
	}
}

func isExpired(lastHeartBeat, cleanThresholdInMinus int64) bool {
	return time.Now().Unix()-lastHeartBeat > cleanThresholdInMinus*60
}

func isEmpty(r ResourceToClean) bool {
	return len(r.ServicesToDelete) == 0 &&
		len(r.PodsToDelete) == 0 &&
		len(r.ConfigMapsToDelete) == 0 &&
		len(r.ServicesToUnlock) == 0 &&
		len(r.ServicesToRecover) == 0 &&
		len(r.DeploymentsToScale) == 0
}
