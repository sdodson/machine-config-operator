package operator

import (
	"encoding/json"
	"fmt"

	"github.com/golang/glog"
	configv1 "github.com/openshift/api/config/v1"
	cov1helpers "github.com/openshift/library-go/pkg/config/clusteroperator/v1helpers"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	ctrlcommon "github.com/openshift/machine-config-operator/pkg/controller/common"
	"github.com/openshift/machine-config-operator/pkg/version"
)

// syncVersion handles reporting the version to the clusteroperator
func (optr *Operator) syncVersion() error {
	co, err := optr.fetchClusterOperator()
	if err != nil {
		return err
	}
	if co == nil {
		return nil
	}

	// keep the old version and progressing if we fail progressing
	if cov1helpers.IsStatusConditionTrue(co.Status.Conditions, configv1.OperatorProgressing) && cov1helpers.IsStatusConditionTrue(co.Status.Conditions, configv1.OperatorFailing) {
		return nil
	}

	co.Status.Versions = optr.vStore.GetAll()
	optr.setMachineConfigPoolStatuses(&co.Status)
	_, err = optr.configClient.ConfigV1().ClusterOperators().UpdateStatus(co)
	return err
}

// syncAvailableStatus applies the new condition to the mco's ClusterOperator object.
func (optr *Operator) syncAvailableStatus() error {
	co, err := optr.fetchClusterOperator()
	if err != nil {
		return err
	}
	if co == nil {
		return nil
	}

	optrVersion, _ := optr.vStore.Get("operator")
	failing := cov1helpers.IsStatusConditionTrue(co.Status.Conditions, configv1.OperatorFailing)
	message := fmt.Sprintf("Cluster has deployed %s", optrVersion)

	available := configv1.ConditionTrue

	if failing {
		available = configv1.ConditionFalse
		message = fmt.Sprintf("Cluster not available for %s", optrVersion)
	}

	// set available
	cov1helpers.SetStatusCondition(&co.Status.Conditions, configv1.ClusterOperatorStatusCondition{
		Type: configv1.OperatorAvailable, Status: available,
		Message: message,
	})

	optr.setMachineConfigPoolStatuses(&co.Status)
	_, err = optr.configClient.ConfigV1().ClusterOperators().UpdateStatus(co)
	return err
}

// syncProgressingStatus applies the new condition to the mco's ClusterOperator object.
func (optr *Operator) syncProgressingStatus() error {
	co, err := optr.fetchClusterOperator()
	if err != nil {
		return err
	}
	if co == nil {
		return nil
	}

	optrVersion, _ := optr.vStore.Get("operator")
	progressing := configv1.ConditionFalse
	message := fmt.Sprintf("Cluster version is %s", optrVersion)

	if optr.vStore.Equal(co.Status.Versions) {
		if optr.inClusterBringup {
			message = fmt.Sprintf("Cluster is bootstrapping %s", optrVersion)
			progressing = configv1.ConditionTrue
		}
	} else {
		message = fmt.Sprintf("Working towards %s", optrVersion)
		progressing = configv1.ConditionTrue
	}

	cov1helpers.SetStatusCondition(&co.Status.Conditions, configv1.ClusterOperatorStatusCondition{
		Type: configv1.OperatorProgressing, Status: progressing,
		Message: message,
	})

	optr.setMachineConfigPoolStatuses(&co.Status)
	_, err = optr.configClient.ConfigV1().ClusterOperators().UpdateStatus(co)
	return err
}

// syncFailingStatus applies the new condition to the mco's ClusterOperator object.
func (optr *Operator) syncFailingStatus(ierr error) (err error) {
	co, err := optr.fetchClusterOperator()
	if err != nil {
		return err
	}
	if co == nil {
		return nil
	}

	optrVersion, _ := optr.vStore.Get("operator")
	failing := configv1.ConditionTrue
	var message, reason string
	if ierr == nil {
		failing = configv1.ConditionFalse
	} else {
		if optr.vStore.Equal(co.Status.Versions) {
			// syncing the state to exiting version.
			message = fmt.Sprintf("Failed to resync %s because: %v", optrVersion, ierr.Error())
		} else {
			message = fmt.Sprintf("Unable to apply %s: %v", optrVersion, ierr.Error())
		}
		reason = ierr.Error()

		// set progressing
		if cov1helpers.IsStatusConditionTrue(co.Status.Conditions, configv1.OperatorProgressing) {
			cov1helpers.SetStatusCondition(&co.Status.Conditions, configv1.ClusterOperatorStatusCondition{Type: configv1.OperatorProgressing, Status: configv1.ConditionTrue, Message: fmt.Sprintf("Unable to apply %s", optrVersion)})
		} else {
			cov1helpers.SetStatusCondition(&co.Status.Conditions, configv1.ClusterOperatorStatusCondition{Type: configv1.OperatorProgressing, Status: configv1.ConditionFalse, Message: fmt.Sprintf("Error while reconciling %s", optrVersion)})
		}
	}
	// set failing condition
	cov1helpers.SetStatusCondition(&co.Status.Conditions, configv1.ClusterOperatorStatusCondition{
		Type: configv1.OperatorFailing, Status: failing,
		Message: message,
		Reason:  reason,
	})

	optr.setMachineConfigPoolStatuses(&co.Status)
	_, err = optr.configClient.ConfigV1().ClusterOperators().UpdateStatus(co)
	return err
}

func (optr *Operator) fetchClusterOperator() (*configv1.ClusterOperator, error) {
	co, err := optr.configClient.ConfigV1().ClusterOperators().Get(optr.name, metav1.GetOptions{})
	if meta.IsNoMatchError(err) {
		return nil, nil
	}
	if apierrors.IsNotFound(err) {
		return optr.initializeClusterOperator()
	}
	if err != nil {
		return nil, err
	}
	return co, nil
}

func (optr *Operator) initializeClusterOperator() (*configv1.ClusterOperator, error) {
	co, err := optr.configClient.ConfigV1().ClusterOperators().Create(&configv1.ClusterOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name: optr.name,
		},
	})
	if err != nil {
		return nil, err
	}
	cov1helpers.SetStatusCondition(&co.Status.Conditions, configv1.ClusterOperatorStatusCondition{Type: configv1.OperatorAvailable, Status: configv1.ConditionFalse})
	cov1helpers.SetStatusCondition(&co.Status.Conditions, configv1.ClusterOperatorStatusCondition{Type: configv1.OperatorProgressing, Status: configv1.ConditionFalse})
	cov1helpers.SetStatusCondition(&co.Status.Conditions, configv1.ClusterOperatorStatusCondition{Type: configv1.OperatorFailing, Status: configv1.ConditionFalse})
	// RelatedObjects are consumed by https://github.com/openshift/must-gather
	co.Status.RelatedObjects = []configv1.ObjectReference{
		{Resource: "namespaces", Name: "openshift-machine-config-operator"},
	}
	// During an installation we report the RELEASE_VERSION as soon as the component is created
	// whether for normal runs and upgrades this code isn't hit and we get the right version every
	// time. This also only contains the operator RELEASE_VERSION when we're here.
	co.Status.Versions = optr.vStore.GetAll()
	return optr.configClient.ConfigV1().ClusterOperators().UpdateStatus(co)
}

func (optr *Operator) setMachineConfigPoolStatuses(status *configv1.ClusterOperatorStatus) {
	statuses, err := optr.allMachineConfigPoolStatus()
	if err != nil {
		glog.Error(err)
		return
	}
	raw, err := json.Marshal(statuses)
	if err != nil {
		glog.Error(err)
		return
	}
	status.Extension.Raw = raw
}

func (optr *Operator) allMachineConfigPoolStatus() (map[string]string, error) {
	pools, err := optr.mcpLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	ret := map[string]string{}
	for _, pool := range pools {
		p := pool.DeepCopy()
		err := isMachineConfigPoolConfigurationValid(p, version.Version.String(), optr.mcLister.Get)
		if err != nil {
			glog.V(4).Infof("Skipping status for pool %s because %v", p.GetName(), err)
			continue
		}
		ret[p.GetName()] = machineConfigPoolStatus(p)
	}
	return ret, nil
}

// isMachineConfigPoolConfigurationValid returns nil error when the configuration of a `pool` is created by the controller at version `version`.
func isMachineConfigPoolConfigurationValid(pool *mcfgv1.MachineConfigPool, version string, machineConfigGetter func(string) (*mcfgv1.MachineConfig, error)) error {
	// both .status.configuration.name and .status.configuration.source must be set.
	if len(pool.Status.Configuration.Name) == 0 {
		return fmt.Errorf("configuration for pool %s is empty", pool.GetName())
	}
	if len(pool.Status.Configuration.Source) == 0 {
		return fmt.Errorf("list of MachineConfigs that were used to generate configuration for pool %s is empty", pool.GetName())
	}

	type configValidationTask struct {
		name                 string
		versionCheckRequired bool
	}
	// we check that all the machineconfigs (generated, and those that were used to create generated) were generated by correct version of the controller.
	tasks := []configValidationTask{{
		name:                 pool.Status.Configuration.Name,
		versionCheckRequired: true,
	}}
	for _, ref := range pool.Status.Configuration.Source {
		tasks = append(tasks, configValidationTask{name: ref.Name, versionCheckRequired: false})
	}
	for _, t := range tasks {
		mc, err := machineConfigGetter(t.name)
		if err != nil {
			return err
		}

		v, ok := mc.Annotations[ctrlcommon.GeneratedByControllerVersionAnnotationKey]
		if t.versionCheckRequired && !ok {
			return fmt.Errorf("%s must be created by controller version %s", t.name, version)
		}
		if ok && v != version {
			return fmt.Errorf("controller version mismatch for %s expected %s has %s", t.name, version, v)
		}
	}
	return nil
}

func machineConfigPoolStatus(pool *mcfgv1.MachineConfigPool) string {
	switch {
	case mcfgv1.IsMachineConfigPoolConditionTrue(pool.Status.Conditions, mcfgv1.MachineConfigPoolUpdated):
		return fmt.Sprintf("all %d nodes are at latest configuration %s", pool.Status.MachineCount, pool.Status.Configuration.Name)
	case mcfgv1.IsMachineConfigPoolConditionTrue(pool.Status.Conditions, mcfgv1.MachineConfigPoolUpdating):
		return fmt.Sprintf("%d out of %d nodes have updated to latest configuration %s", pool.Status.UpdatedMachineCount, pool.Status.MachineCount, pool.Status.Configuration.Name)
	default:
		return "<unknown>"
	}
}
