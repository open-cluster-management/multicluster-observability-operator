// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"context"
	"errors"
	"reflect"

	"github.com/go-logr/logr"
	operatorv1 "github.com/openshift/api/operator/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	addonv1alpha1 "github.com/open-cluster-management/api/addon/v1alpha1"
	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	workv1 "github.com/open-cluster-management/api/work/v1"
	mcov1beta1 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	mchv1 "github.com/open-cluster-management/multiclusterhub-operator/pkg/apis/operator/v1"
)

const (
	ownerLabelKey   = "owner"
	ownerLabelValue = "multicluster-observability-operator"
	certsName       = "observability-managed-cluster-certs"
)

var (
	log                             = logf.Log.WithName("controller_placementrule")
	watchNamespace                  = config.GetDefaultNamespace()
	isCRoleCreated                  = false
	isClusterManagementAddonCreated = false
	isplacementControllerRunnning   = false
	managedClusterList              = []string{}
)

// PlacementRuleReconciler reconciles a PlacementRule object
type PlacementRuleReconciler struct {
	Client     client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
	CRDMap     map[string]bool
	APIReader  client.Reader
	RESTMapper meta.RESTMapper
}

// +kubebuilder:rbac:groups=observability.open-cluster-management.io,resources=placementrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=observability.open-cluster-management.io,resources=placementrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=observability.open-cluster-management.io,resources=placementrules/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Modify the Reconcile function to compare the state specified by
// the PlacementRule object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.0/pkg/reconcile
func (r *PlacementRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	reqLogger.Info("Reconciling PlacementRule")

	if config.GetMonitoringCRName() == "" {
		reqLogger.Info("multicluster observability resource is not available")
		return ctrl.Result{}, nil
	}

	deleteAll := false
	// Fetch the MultiClusterObservability instance
	mco := &mcov1beta2.MultiClusterObservability{}
	err := r.Client.Get(context.TODO(),
		types.NamespacedName{
			Name: config.GetMonitoringCRName(),
		}, mco)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			deleteAll = true
		} else {
			// Error reading the object - requeue the request.
			return ctrl.Result{}, err
		}
	}

	// Do not reconcile objects if this instance of mch is labeled "paused"
	if config.IsPaused(mco.GetAnnotations()) {
		reqLogger.Info("MCO reconciliation is paused. Nothing more to do.")
		return ctrl.Result{}, nil
	}

	//read image manifest configmap to be used to replace the image for each component.
	mchCrdExists, _ := r.CRDMap[config.MCHCrdName]
	if req.Name == config.MCHUpdatedRequestName && mchCrdExists {
		mchList := &mchv1.MultiClusterHubList{}
		mchistOpts := []client.ListOption{
			client.InNamespace(config.GetMCONamespace()),
		}
		err := r.Client.List(context.TODO(), mchList, mchistOpts...)
		if err != nil {
			return ctrl.Result{}, err
		}

		// normally there should only one MCH CR in the cluster
		if len(mchList.Items) == 1 {
			mch := mchList.Items[0]
			if mch.Status.CurrentVersion == mch.Status.DesiredVersion && mch.Status.CurrentVersion != "" {
				mchVer := mch.Status.CurrentVersion
				//read image manifest configmap to be used to replace the image for each component.
				if _, err = config.ReadImageManifestConfigMap(r.Client, mchVer); err != nil {
					return ctrl.Result{}, err
				}
			}
		}
	}

	opts := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{ownerLabelKey: ownerLabelValue}),
	}
	if req.Namespace != config.GetDefaultNamespace() &&
		req.Namespace != "" {
		opts.Namespace = req.Namespace
	}

	obsAddonList := &mcov1beta1.ObservabilityAddonList{}
	err = r.Client.List(context.TODO(), obsAddonList, opts)
	if err != nil {
		reqLogger.Error(err, "Failed to list observabilityaddon resource")
		return ctrl.Result{}, err
	}

	if !deleteAll {
		res, err := createAllRelatedRes(r.Client, r.RESTMapper, req, mco, obsAddonList)
		if err != nil {
			return res, err
		}
	} else {
		res, err := deleteAllObsAddons(r.Client, obsAddonList)
		if err != nil {
			return res, err
		}
	}

	obsAddonList = &mcov1beta1.ObservabilityAddonList{}
	err = r.Client.List(context.TODO(), obsAddonList, opts)
	if err != nil {
		reqLogger.Error(err, "Failed to list observabilityaddon resource")
		return ctrl.Result{}, err
	}
	workList := &workv1.ManifestWorkList{}
	err = r.Client.List(context.TODO(), workList, opts)
	if err != nil {
		reqLogger.Error(err, "Failed to list manifestwork resource")
		return ctrl.Result{}, err
	}
	latestClusters := []string{}
	staleAddons := []string{}
	for _, addon := range obsAddonList.Items {
		latestClusters = append(latestClusters, addon.Namespace)
		staleAddons = append(staleAddons, addon.Namespace)
	}
	for _, work := range workList.Items {
		if work.Name != work.Namespace+workNameSuffix {
			reqLogger.Info("To delete invalid manifestwork", "name", work.Name, "namespace", work.Namespace)
			err = deleteManifestWork(r.Client, work.Name, work.Namespace)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		if !util.Contains(latestClusters, work.Namespace) {
			reqLogger.Info("To delete manifestwork", "namespace", work.Namespace)
			err = deleteManagedClusterRes(r.Client, work.Namespace)
			if err != nil {
				return ctrl.Result{}, err
			}
		} else {
			staleAddons = util.Remove(staleAddons, work.Namespace)
		}
	}

	// delete stale addons if manifestwork does not exist
	for _, addon := range staleAddons {
		err = deleteStaleObsAddon(r.Client, addon, true)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// only update managedclusteraddon status when obs addon's status updated
	if req.Name == obsAddonName {
		err = updateAddonStatus(r.Client, *obsAddonList)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if deleteAll {
		opts.Namespace = ""
		err = r.Client.List(context.TODO(), workList, opts)
		if err != nil {
			reqLogger.Error(err, "Failed to list manifestwork resource")
			return ctrl.Result{}, err
		}
		if len(workList.Items) == 0 {
			err = deleteGlobalResource(r.Client)
		}
	}

	return ctrl.Result{}, err
}

func createAllRelatedRes(
	c client.Client,
	restMapper meta.RESTMapper,
	request ctrl.Request,
	mco *mcov1beta2.MultiClusterObservability,
	obsAddonList *mcov1beta1.ObservabilityAddonList) (ctrl.Result, error) {

	// create the clusterrole if not there
	if !isCRoleCreated {
		err := createClusterRole(c)
		if err != nil {
			return ctrl.Result{}, err
		}
		err = createResourceRole(c)
		if err != nil {
			return ctrl.Result{}, err
		}
		isCRoleCreated = true
	}
	//Check if ClusterManagementAddon is created or create it
	if !isClusterManagementAddonCreated {
		err := util.CreateClusterManagementAddon(c)
		if err != nil {
			return ctrl.Result{}, err
		}
		isClusterManagementAddonCreated = true
	}

	currentClusters := []string{}
	for _, ep := range obsAddonList.Items {
		currentClusters = append(currentClusters, ep.Namespace)
	}

	works, crdWork, dep, hubInfo, err := getGlobalManifestResources(c, mco)
	if err != nil {
		return ctrl.Result{}, err
	}

	failedCreateManagedClusterRes := false
	for _, managedCluster := range managedClusterList {
		currentClusters = util.Remove(currentClusters, managedCluster)
		// only handle the request namespace if the request resource is not from observability  namespace
		if request.Namespace == "" || request.Namespace == config.GetDefaultNamespace() ||
			request.Namespace == managedCluster {
			log.Info("Monitoring operator should be installed in cluster", "cluster_name", managedCluster)
			err = createManagedClusterRes(c, restMapper, mco,
				managedCluster, managedCluster,
				works, crdWork, dep, hubInfo)
			if err != nil {
				failedCreateManagedClusterRes = true
				log.Error(err, "Failed to create managedcluster resources", "namespace", managedCluster)
			}
			if request.Namespace == managedCluster {
				break
			}
		}
	}

	failedDeleteOba := false
	for _, cluster := range currentClusters {
		log.Info("To delete observabilityAddon", "namespace", cluster)
		err = deleteObsAddon(c, cluster)
		if err != nil {
			failedDeleteOba = true
			log.Error(err, "Failed to delete observabilityaddon", "namespace", cluster)
		}
	}

	if failedCreateManagedClusterRes || failedDeleteOba {
		return ctrl.Result{}, errors.New("Failed to create managedcluster resources or" +
			" failed to delete observabilityaddon, skip and reconcile later")
	}

	return ctrl.Result{}, nil
}

func deleteAllObsAddons(
	client client.Client,
	obsAddonList *mcov1beta1.ObservabilityAddonList) (ctrl.Result, error) {
	for _, ep := range obsAddonList.Items {
		err := deleteObsAddon(client, ep.Namespace)
		if err != nil {
			log.Error(err, "Failed to delete observabilityaddon", "namespace", ep.Namespace)
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func deleteGlobalResource(c client.Client) error {
	err := deleteClusterRole(c)
	if err != nil {
		return err
	}
	err = deleteResourceRole(c)
	if err != nil {
		return err
	}
	isCRoleCreated = false
	//delete ClusterManagementAddon
	err = util.DeleteClusterManagementAddon(c)
	if err != nil {
		return err
	}
	isClusterManagementAddonCreated = false
	return nil
}

func createManagedClusterRes(client client.Client, restMapper meta.RESTMapper,
	mco *mcov1beta2.MultiClusterObservability, name string, namespace string,
	works []workv1.Manifest, crdWork *workv1.Manifest, dep *appsv1.Deployment, hubInfo *corev1.Secret) error {
	err := createObsAddon(client, namespace)
	if err != nil {
		log.Error(err, "Failed to create observabilityaddon")
		return err
	}

	err = createRolebindings(client, namespace, name)
	if err != nil {
		return err
	}

	err = createManifestWorks(client, restMapper, namespace, name, mco, works, crdWork, dep, hubInfo)
	if err != nil {
		log.Error(err, "Failed to create manifestwork")
		return err
	}

	err = util.CreateManagedClusterAddonCR(client, namespace)
	if err != nil {
		log.Error(err, "Failed to create ManagedClusterAddon")
		return err
	}

	return nil
}

func deleteManagedClusterRes(c client.Client, namespace string) error {

	managedclusteraddon := &addonv1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.ManagedClusterAddonName,
			Namespace: namespace,
		},
	}
	err := c.Delete(context.TODO(), managedclusteraddon)
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}

	err = deleteRolebindings(c, namespace)
	if err != nil {
		return err
	}

	err = deleteManifestWorks(c, namespace)
	if err != nil {
		log.Error(err, "Failed to delete manifestwork")
		return err
	}
	return nil
}

func updateManagedClusterList(obj client.Object) {
	vendor, ok := obj.GetLabels()["vendor"]
	if !ok && vendor != "OpenShift" {
		return
	}
	obs, ok := obj.GetLabels()["observability"]
	if ok && obs == "disabled" {
		return
	}
	managedClusterList = util.RemoveDuplicates(append(managedClusterList, obj.GetName()))
}

// SetupWithManager sets up the controller with the Manager.
func (r *PlacementRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	clusterPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			updateManagedClusterList(e.Object)
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				updateManagedClusterList(e.ObjectNew)
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			updateManagedClusterList(e.Object)
			return e.DeleteStateUnknown
		},
	}

	obsAddonPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetName() == obsAddonName &&
				e.ObjectNew.GetLabels()[ownerLabelKey] == ownerLabelValue &&
				!reflect.DeepEqual(e.ObjectNew.(*mcov1beta1.ObservabilityAddon).Status.Conditions,
					e.ObjectOld.(*mcov1beta1.ObservabilityAddon).Status.Conditions) {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetName() == obsAddonName &&
				e.Object.GetLabels()[ownerLabelKey] == ownerLabelValue {
				return true
			}
			return false
		},
	}

	mcoPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			// only reconcile when ObservabilityAddonSpec updated
			if e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() &&
				!reflect.DeepEqual(e.ObjectNew.(*mcov1beta2.MultiClusterObservability).Spec.ObservabilityAddonSpec,
					e.ObjectOld.(*mcov1beta2.MultiClusterObservability).Spec.ObservabilityAddonSpec) {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return true
		},
	}

	customAllowlistPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetName() == config.AllowlistCustomConfigMapName &&
				e.Object.GetNamespace() == config.GetDefaultNamespace() {
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetName() == config.AllowlistCustomConfigMapName &&
				e.ObjectNew.GetNamespace() == config.GetDefaultNamespace() &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetName() == config.AllowlistCustomConfigMapName &&
				e.Object.GetNamespace() == config.GetDefaultNamespace() {
				return true
			}
			return false
		},
	}

	certSecretPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetName() == config.ServerCACerts &&
				e.Object.GetNamespace() == config.GetDefaultNamespace() {
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if (e.ObjectNew.GetName() == config.ServerCACerts &&
				e.ObjectNew.GetNamespace() == config.GetDefaultNamespace()) &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}

	ingressControllerPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetName() == config.OpenshiftIngressOperatorCRName &&
				e.Object.GetNamespace() == config.OpenshiftIngressOperatorNamespace {
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetName() == config.OpenshiftIngressOperatorCRName &&
				e.ObjectNew.GetNamespace() == config.OpenshiftIngressOperatorNamespace {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetName() == config.OpenshiftIngressOperatorCRName &&
				e.Object.GetNamespace() == config.OpenshiftIngressOperatorNamespace {
				return true
			}
			return false
		},
	}

	amRouterCertSecretPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetNamespace() == config.GetDefaultNamespace() &&
				(e.Object.GetName() == config.AlertmanagerRouteBYOCAName ||
					e.Object.GetName() == config.AlertmanagerRouteBYOCERTName) {
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetNamespace() == config.GetDefaultNamespace() &&
				(e.ObjectNew.GetName() == config.AlertmanagerRouteBYOCAName ||
					e.ObjectNew.GetName() == config.AlertmanagerRouteBYOCERTName) {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetNamespace() == config.GetDefaultNamespace() &&
				(e.Object.GetName() == config.AlertmanagerRouteBYOCAName ||
					e.Object.GetName() == config.AlertmanagerRouteBYOCERTName) {
				return true
			}
			return false
		},
	}

	routeCASecretPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if (e.Object.GetNamespace() == config.OpenshiftIngressOperatorNamespace &&
				e.Object.GetName() == config.OpenshiftIngressRouteCAName) ||
				(e.Object.GetNamespace() == config.OpenshiftIngressNamespace &&
					e.Object.GetName() == config.OpenshiftIngressDefaultCertName) {
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if ((e.ObjectNew.GetNamespace() == config.OpenshiftIngressOperatorNamespace &&
				e.ObjectNew.GetName() == config.OpenshiftIngressRouteCAName) ||
				(e.ObjectNew.GetNamespace() == config.OpenshiftIngressNamespace &&
					e.ObjectNew.GetName() == config.OpenshiftIngressDefaultCertName)) &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}

	amAccessorSAPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetName() == config.AlertmanagerAccessorSAName &&
				e.Object.GetNamespace() == config.GetDefaultNamespace() {
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if (e.ObjectNew.GetName() == config.AlertmanagerAccessorSAName &&
				e.ObjectNew.GetNamespace() == config.GetDefaultNamespace()) &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}

	ctrBuilder := ctrl.NewControllerManagedBy(mgr).
		// Watch for changes to primary resource ManagedCluster with predicate
		For(&clusterv1.ManagedCluster{}, builder.WithPredicates(clusterPred)).
		// secondary watch for observabilityaddon
		Watches(&source.Kind{Type: &mcov1beta1.ObservabilityAddon{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(obsAddonPred)).
		// secondary watch for MCO
		Watches(&source.Kind{Type: &mcov1beta2.MultiClusterObservability{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(mcoPred)).
		// secondary watch for custom allowlist configmap
		Watches(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(customAllowlistPred)).
		// secondary watch for certificate secrets
		Watches(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(certSecretPred)).
		// secondary watch for default ingresscontroller
		Watches(&source.Kind{Type: &operatorv1.IngressController{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(ingressControllerPred)).
		// secondary watch for alertmanager route byo cert secrets
		Watches(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(amRouterCertSecretPred)).
		// secondary watch for openshift route ca secret
		Watches(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(routeCASecretPred)).
		// secondary watch for alertmanager accessor serviceaccount
		Watches(&source.Kind{Type: &corev1.ServiceAccount{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(amAccessorSAPred))

	manifestWorkGroupKind := schema.GroupKind{Group: workv1.GroupVersion.Group, Kind: "ManifestWork"}
	if _, err := r.RESTMapper.RESTMapping(manifestWorkGroupKind, workv1.GroupVersion.Version); err == nil {
		workPred := predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return false
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				if e.ObjectNew.GetLabels()[ownerLabelKey] == ownerLabelValue &&
					e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() &&
					!reflect.DeepEqual(e.ObjectNew.(*workv1.ManifestWork).Spec.Workload.Manifests,
						e.ObjectOld.(*workv1.ManifestWork).Spec.Workload.Manifests) {
					return true
				}
				return false
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return e.Object.GetLabels()[ownerLabelKey] == ownerLabelValue
			},
		}

		// secondary watch for manifestwork
		ctrBuilder = ctrBuilder.Watches(&source.Kind{Type: &workv1.ManifestWork{}}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(workPred))
	}

	mchGroupKind := schema.GroupKind{Group: mchv1.SchemeGroupVersion.Group, Kind: "MultiClusterHub"}
	if _, err := r.RESTMapper.RESTMapping(mchGroupKind, mchv1.SchemeGroupVersion.Version); err == nil {
		mchPred := predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return true
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				if e.ObjectNew.GetNamespace() == config.GetMCONamespace() &&
					e.ObjectNew.(*mchv1.MultiClusterHub).Status.DesiredVersion == e.ObjectNew.(*mchv1.MultiClusterHub).Status.CurrentVersion {
					// only enqueue the request when the MCH is installed/upgraded successfully
					return true
				}
				return false
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return false
			},
		}

		mchCrdExists, _ := r.CRDMap[config.MCHCrdName]
		if mchCrdExists {
			// secondary watch for MCH
			ctrBuilder = ctrBuilder.Watches(&source.Kind{Type: &mchv1.MultiClusterHub{}}, handler.EnqueueRequestsFromMapFunc(func(a client.Object) []reconcile.Request {
				return []reconcile.Request{
					{NamespacedName: types.NamespacedName{
						Name:      config.MCHUpdatedRequestName,
						Namespace: a.GetNamespace(),
					}},
				}
			}), builder.WithPredicates(mchPred))
		}
	}

	// create and return a new controller
	return ctrBuilder.Complete(r)
}

func StartPlacementController(mgr manager.Manager, crdMap map[string]bool) error {
	if isplacementControllerRunnning {
		return nil
	}
	isplacementControllerRunnning = true

	if err := (&PlacementRuleReconciler{
		Client:     mgr.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName("PlacementRule"),
		Scheme:     mgr.GetScheme(),
		APIReader:  mgr.GetAPIReader(),
		CRDMap:     crdMap,
		RESTMapper: mgr.GetRESTMapper(),
	}).SetupWithManager(mgr); err != nil {
		log.Error(err, "unable to create controller", "controller", "PlacementRule")
		return err
	}

	return nil
}
