package jupyterlab

import (
	"context"
	"reflect"
	jupyterv1alpha1 "github.com/atef23/jupyter-lab-operator/pkg/apis/jupyter/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	// route imports
	routev1 "github.com/openshift/api/route/v1"
	"github.com/atef23/jupyter-lab-operator/pkg/controller/ocp"
	"fmt"
)

var log = logf.Log.WithName("controller_jupyterlab")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new JupyterLab Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {

	if err := routev1.AddToScheme(mgr.GetScheme()); err != nil {
		return err
	}

	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileJupyterLab{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("jupyterlab-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource JupyterLab
	err = c.Watch(&source.Kind{Type: &jupyterv1alpha1.JupyterLab{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner JupyterLab
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &jupyterv1alpha1.JupyterLab{},
	})
	if err != nil {
		return err
	}

	// watch for Route only on OpenShift
	if err = c.Watch(&source.Kind{Type: &routev1.Route{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &jupyterv1alpha1.JupyterLab{},
	}); err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileJupyterLab implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileJupyterLab{}

// ReconcileJupyterLab reconciles a JupyterLab object
type ReconcileJupyterLab struct {
	// TODO: Clarify the split client
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the jupyter and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a JupyterLab object and makes changes based on the state read
// and what is in the JupyterLab.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a JupyterLab Deployment for each JupyterLab CR
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileJupyterLab) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling JupyterLab")

	// Fetch the JupyterLab instance
	jupyterLab := &jupyterv1alpha1.JupyterLab{}
	err := r.client.Get(context.TODO(), request.NamespacedName, jupyterLab)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("JupyterLab resource not found. Ignoring since object must be deleted")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		reqLogger.Error(err, "Failed to get JupyterLab")
		return reconcile.Result{}, err
	}

	// Check if the deployment already exists, if not create a new one
	found := &appsv1.Deployment{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: jupyterLab.Name, Namespace: jupyterLab.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {

		// Define a new service
		service := r.serviceForJupyterLab(jupyterLab)
		reqLogger.Info("Creating a new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
		err = r.client.Create(context.TODO(), service)
		if err != nil {
			reqLogger.Error(err, "Failed to create new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
			return reconcile.Result{}, err
		}

		// define a new route
		route := ocp.NewRoute(jupyterLab.Name, jupyterLab.Namespace, fmt.Sprintf("%s-server", jupyterLab.Name), 8888);
		reqLogger.Info("Creating a new Route", "Route.Namespace", route.Namespace, "Route.Name", route.Name)
		controllerutil.SetControllerReference(jupyterLab, route, r.scheme)
		if err := r.client.Create(context.TODO(), route); err != nil {
			reqLogger.Error(err, "Failed to create new Route", "Route.Namespace", route.Namespace, "Route.Name", route.Name)
			return reconcile.Result{}, err
		}

		// Define a new deployment
		dep := r.deploymentForJupyterLab(jupyterLab)
		reqLogger.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		err = r.client.Create(context.TODO(), dep)
		if err != nil {
			reqLogger.Error(err, "Failed to create new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return reconcile.Result{}, err
		}
		// Deployment created successfully - return and requeue
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get Deployment")
		return reconcile.Result{}, err
	}


	// Ensure the deployment size is the same as the spec
	size := jupyterLab.Spec.Size
	if *found.Spec.Replicas != size {
		found.Spec.Replicas = &size
		err = r.client.Update(context.TODO(), found)
		if err != nil {
			reqLogger.Error(err, "Failed to update Deployment", "Deployment.Namespace", found.Namespace, "Deployment.Name", found.Name)
			return reconcile.Result{}, err
		}
		// Spec updated - return and requeue
		return reconcile.Result{Requeue: true}, nil
	}

	// Update the JupyterLab status with the pod names
	// List the pods for this jupyterLab's deployment
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(jupyterLab.Namespace),
		client.MatchingLabels(labelsForJupyterLab(jupyterLab.Name)),
	}
	if err = r.client.List(context.TODO(), podList, listOpts...); err != nil {
		reqLogger.Error(err, "Failed to list pods", "JupyterLab.Namespace", jupyterLab.Namespace, "JupyterLab.Name", jupyterLab.Name)
		return reconcile.Result{}, err
	}
	podNames := getPodNames(podList.Items)

	// Update status.Nodes if needed
	if !reflect.DeepEqual(podNames, jupyterLab.Status.Nodes) {
		jupyterLab.Status.Nodes = podNames
		err := r.client.Status().Update(context.TODO(), jupyterLab)
		if err != nil {
			reqLogger.Error(err, "Failed to update JupyterLab status")
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

// deploymentForJupyterLab returns a jupyterLab Deployment object
func (r *ReconcileJupyterLab) deploymentForJupyterLab(m *jupyterv1alpha1.JupyterLab) *appsv1.Deployment {
	ls := labelsForJupyterLab(m.Name)
	replicas := m.Spec.Size

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "jupyterlab",
							Image:   "quay.io/aaziz/jupyterlab:latest",
							Ports: []corev1.ContainerPort{
								{
									Protocol:      corev1.ProtocolTCP,
									ContainerPort: 8888,
								},
							},

						},
					},
				},
			},
		},
	}
	// Set JupyterLab instance as the owner and controller
	controllerutil.SetControllerReference(m, dep, r.scheme)
	return dep
}

// labelsForJupyterLab returns the labels for selecting the resources
// belonging to the given jupyterLab CR name.
func labelsForJupyterLab(name string) map[string]string {
	return map[string]string{"app": "jupyterLab", "jupyterLab_cr": name}
}

func selectorsForService(name string) map[string]string {
	return map[string]string{
		"app": "jupyterLab",
	}
}

// getPodNames returns the pod names of the array of pods passed in
func getPodNames(pods []corev1.Pod) []string {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return podNames
}

// serviceForJupyterLab returns a jupyterLab Service object
func (r *ReconcileJupyterLab) serviceForJupyterLab(m *jupyterv1alpha1.JupyterLab) *corev1.Service {
	selectors := selectorsForService(m.Name)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
		},
	}

	service.Spec = corev1.ServiceSpec{
		Ports:     jupyterNoteBooksPort.asServicePorts(),
		Selector:  selectors,
	}

	// Set JupyterLab instance as the owner and controller
	controllerutil.SetControllerReference(m, service, r.scheme)
	return service
}