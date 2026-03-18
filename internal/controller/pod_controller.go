package controller

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var (
	managedPods = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "envoy_router",
		Name:      "managed_pods",
		Help:      "Number of pods currently managed by envoy-router (have the finalizer).",
	})
)

func init() {
	metrics.Registry.MustRegister(managedPods)
}

const (
	managedByLabel = "app.kubernetes.io/managed-by"
	managedByValue = "envoy-router"
	enabledLabel   = "envoy-router/enabled"
	finalizerName  = "envoy-router.comet.ml/finalizer"
)

// PodReconciler reconciles Pods labelled with envoy-router/enabled=true.
// For each such pod it maintains a selector-less Service, an EndpointSlice,
// and an HTTPRoute that maps /<pod-name> to the pod's IP.
//
// The HTTPRoute's ParentReference always points to a Gateway in the same
// namespace as the pod, enabling one isolated Gateway per namespace.
type PodReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	GatewayName string
	ServicePort int32
	PodPort     int32
}

func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	pod := &corev1.Pod{}
	if err := r.Get(ctx, req.NamespacedName, pod); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if pod.Labels[enabledLabel] != "true" {
		return ctrl.Result{}, nil
	}

	if !pod.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(pod, finalizerName) {
			if err := r.cleanup(ctx, pod); err != nil {
				return ctrl.Result{}, err
			}
			patch := client.MergeFrom(pod.DeepCopy())
			controllerutil.RemoveFinalizer(pod, finalizerName)
			managedPods.Dec()
			return ctrl.Result{}, r.Patch(ctx, pod, patch)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(pod, finalizerName) {
		patch := client.MergeFrom(pod.DeepCopy())
		controllerutil.AddFinalizer(pod, finalizerName)
		if err := r.Patch(ctx, pod, patch); err != nil {
			return ctrl.Result{}, err
		}
		managedPods.Inc()
	}

	if pod.Status.PodIP == "" {
		log.Info("pod IP not yet assigned, waiting for status update")
		return ctrl.Result{}, nil
	}

	if err := r.ensureService(ctx, pod); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.ensureEndpointSlice(ctx, pod); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.ensureHTTPRoute(ctx, pod); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *PodReconciler) cleanup(ctx context.Context, pod *corev1.Pod) error {
	nn := types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}

	route := &gatewayv1.HTTPRoute{}
	if err := r.Get(ctx, nn, route); err == nil {
		if err := r.Delete(ctx, route); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	eps := &discoveryv1.EndpointSlice{}
	if err := r.Get(ctx, nn, eps); err == nil {
		if err := r.Delete(ctx, eps); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	svc := &corev1.Service{}
	if err := r.Get(ctx, nn, svc); err == nil {
		if err := r.Delete(ctx, svc); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

func (r *PodReconciler) ensureService(ctx context.Context, pod *corev1.Pod) error {
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Labels:    map[string]string{managedByLabel: managedByValue},
		},
		Spec: corev1.ServiceSpec{
			// No Selector — traffic is directed via manually managed Endpoints.
			Ports: []corev1.ServicePort{
				{
					Port:       r.ServicePort,
					TargetPort: intstr.FromInt32(r.PodPort),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	existing.Spec.Ports = desired.Spec.Ports
	return r.Update(ctx, existing)
}

func (r *PodReconciler) ensureEndpointSlice(ctx context.Context, pod *corev1.Pod) error {
	ready := true
	port := r.PodPort
	protocol := corev1.ProtocolTCP
	addrType := discoveryv1.AddressTypeIPv4

	desired := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Labels: map[string]string{
				managedByLabel:                   managedByValue,
				discoveryv1.LabelServiceName:     pod.Name,
			},
		},
		AddressType: addrType,
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses:  []string{pod.Status.PodIP},
				Conditions: discoveryv1.EndpointConditions{Ready: &ready},
			},
		},
		Ports: []discoveryv1.EndpointPort{
			{Port: &port, Protocol: &protocol},
		},
	}

	existing := &discoveryv1.EndpointSlice{}
	err := r.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	existing.Endpoints = desired.Endpoints
	existing.Ports = desired.Ports
	return r.Update(ctx, existing)
}

func (r *PodReconciler) ensureHTTPRoute(ctx context.Context, pod *corev1.Pod) error {
	pathPrefix := "/" + pod.Name
	pathType := gatewayv1.PathMatchPathPrefix
	gwNamespace := gatewayv1.Namespace(pod.Namespace)
	port := gatewayv1.PortNumber(r.ServicePort)
	weight := int32(1)

	desired := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Labels:    map[string]string{managedByLabel: managedByValue},
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					{
						Name:      gatewayv1.ObjectName(r.GatewayName),
						Namespace: &gwNamespace,
					},
				},
			},
			Rules: []gatewayv1.HTTPRouteRule{
				{
					Matches: []gatewayv1.HTTPRouteMatch{
						{
							Path: &gatewayv1.HTTPPathMatch{
								Type:  &pathType,
								Value: &pathPrefix,
							},
						},
					},
					BackendRefs: []gatewayv1.HTTPBackendRef{
						{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: gatewayv1.ObjectName(pod.Name),
									Port: &port,
								},
								Weight: &weight,
							},
						},
					},
				},
			},
		},
	}

	existing := &gatewayv1.HTTPRoute{}
	err := r.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	existing.Spec = desired.Spec
	return r.Update(ctx, existing)
}

func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}, builder.WithPredicates(
			predicate.NewPredicateFuncs(func(obj client.Object) bool {
				return obj.GetLabels()[enabledLabel] == "true"
			}),
		)).
		Complete(r)
}
