package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	mygroupv1alpha1 "my-operator/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AppInstanceReconciler reconciles a AppInstance object
type AppInstanceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=mygroup.mydomain.com,resources=appinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mygroup.mydomain.com,resources=appinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mygroup.mydomain.com,resources=appinstances/finalizers,verbs=update

func (r *AppInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the AppInstance resource
	app, err := r.getAppInstance(ctx, req)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Get the list of pods
	podList, err := r.getPodList(ctx, req, app)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Scale the pods
	if err := r.scalePods(ctx, req, app, podList, log); err != nil {
		return ctrl.Result{}, err
	}

	// Update the status
	if err := r.updateStatus(ctx, app, podList, log); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *AppInstanceReconciler) getAppInstance(ctx context.Context, req ctrl.Request) (*mygroupv1alpha1.AppInstance, error) {
	var app mygroupv1alpha1.AppInstance
	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
		return nil, err
	}
	return &app, nil
}

func (r *AppInstanceReconciler) getPodList(ctx context.Context, req ctrl.Request, app *mygroupv1alpha1.AppInstance) (*corev1.PodList, error) {
	var podList corev1.PodList
	opts := []client.ListOption{
		client.InNamespace(req.Namespace),
		client.MatchingLabels{"app": app.Name},
	}
	if err := r.List(ctx, &podList, opts...); err != nil {
		return nil, err
	}
	return &podList, nil
}

func (r *AppInstanceReconciler) scalePods(ctx context.Context, req ctrl.Request, app *mygroupv1alpha1.AppInstance, podList *corev1.PodList, log logr.Logger) error {
	desiredSize := app.Spec.Size
	currentSize := len(podList.Items)

	if currentSize < desiredSize {
		return r.createPods(ctx, req, app, currentSize, desiredSize, log)
	} else if currentSize > desiredSize {
		return r.deletePods(ctx, podList, currentSize, desiredSize, log)
	}
	return nil
}

func (r *AppInstanceReconciler) createPods(ctx context.Context, req ctrl.Request, app *mygroupv1alpha1.AppInstance, currentSize, desiredSize int, log logr.Logger) error {
	for i := currentSize; i < desiredSize; i++ {
		pod := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-pod-%d", app.Name, i),
				Namespace: req.Namespace,
				Labels:    map[string]string{"app": app.Name},
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(app, mygroupv1alpha1.GroupVersion.WithKind("AppInstance")),
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "busybox",
						Image: "busybox",
						Command: []string{
							"sleep", "3600",
						},
					},
				},
			},
		}
		if err := r.Create(ctx, &pod); err != nil {
			log.Error(err, "Failed to create pod")
			return err
		}
	}
	return nil
}

func (r *AppInstanceReconciler) deletePods(ctx context.Context, podList *corev1.PodList, currentSize, desiredSize int, log logr.Logger) error {
	for i := currentSize - 1; i >= desiredSize; i-- {
		if err := r.Delete(ctx, &podList.Items[i]); err != nil {
			log.Error(err, "Failed to delete pod")
			return err
		}
	}
	return nil
}

func (r *AppInstanceReconciler) updateStatus(ctx context.Context, app *mygroupv1alpha1.AppInstance, podList *corev1.PodList, log logr.Logger) error {
	var podNames []string
	for _, pod := range podList.Items {
		podNames = append(podNames, pod.Name)
	}
	app.Status.Nodes = podNames
	if err := r.Status().Update(ctx, app); err != nil {
		log.Error(err, "Failed to update status")
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AppInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mygroupv1alpha1.AppInstance{}).
		Owns(&corev1.Pod{}). // Add this line to watch Pod resources
		Named("appinstance").
		Complete(r)

}
