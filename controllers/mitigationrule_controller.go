/*
Copyright 2020 mmmknt.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	networkingv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	istiocli "istio.io/client-go/pkg/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	spikemitigationv1 "github.com/mmmknt/spike-mitigation-operator/api/v1"
)

// MitigationRuleReconciler reconciles a MitigationRule object
type MitigationRuleReconciler struct {
	client.Client
	IstioClientset *istiocli.Clientset
	Calculator     *MitigationCalculator
	Log            logr.Logger
	Scheme         *runtime.Scheme
}

// +kubebuilder:rbac:groups=spike-mitigation.mmmknt.dev,resources=mitigationrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=spike-mitigation.mmmknt.dev,resources=mitigationrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices,verbs=list;create;update;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=list
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=list;watch

func (r *MitigationRuleReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("mitigationrule", req.NamespacedName)

	log.Info("start Reconcile")
	mitigationRule := &spikemitigationv1.MitigationRule{}
	if err := r.Get(ctx, req.NamespacedName, mitigationRule); err != nil {
		log.Error(err, "unable to get mitigation rule")
		return ctrl.Result{}, err
	}

	// debug
	log.Info("succeed to get mitigation rule", "mitigation rule", mitigationRule)

	defaultNamespace := "default"
	vsList, err := r.IstioClientset.NetworkingV1alpha3().VirtualServices(defaultNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Error(err, "unable to get VirtualService list")
		return ctrl.Result{}, err
	}
	log.Info("succeed to get VirtualService List", "VirtualService", vsList)
	rm := make(map[Host]*RoutingRate)
	for i := range vsList.Items {
		item := vsList.Items[i]
		for ori := range item.OwnerReferences {
			if item.OwnerReferences[ori].Kind == "MitigationRule" {
				internalHost := item.ObjectMeta.GetLabels()["InternalHost"]
				externalHost := item.ObjectMeta.GetLabels()["ExternalHost"]
				spec := item.Spec
				host := spec.GetHosts()[0]
				var iw int32
				var ew int32
				for _, ri := range spec.GetHttp()[0].GetRoute() {
					dw := ri.GetWeight()
					dh := ri.Destination.Host
					if dh == internalHost {
						iw = dw
					} else if dh == externalHost {
						ew = dw
					}
				}
				rm[Host(host)] = &RoutingRate{
					InternalWeight: iw,
					ExternalWeight: ew,
					Version:        item.ObjectMeta.ResourceVersion,
				}
				break
			}
		}
	}
	currentRR := &RoutingRule{RuleMap: rm}

	routingRule, err := r.Calculator.Calculate(ctx, r.Log, currentRR, mitigationRule.Spec)
	log.Info("current", "routing rule", currentRR)
	log.Info("latest", "routing rule", routingRule)
	if err != nil {
		log.Error(err, "unable to calculate routing rule")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	log.Info("succeed to get routing rule", "routing rule", routingRule)
	if currentRR.Equal(routingRule) {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// TODO apply routing rule
	// TODO set labels, internal host and external host values, to VirtualService's meta
	r.apply(mitigationRule, currentRR, routingRule)

	// TODO make RequeueAfter to be able change per loop
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *MitigationRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&spikemitigationv1.MitigationRule{}).
		Complete(r)
}

func (r *MitigationRuleReconciler) apply(mitigationRule *spikemitigationv1.MitigationRule, current, latest *RoutingRule) error {
	creates := make(map[Host]*RoutingRate)
	updates := make(map[Host]*RoutingRate)
	deletes := make(map[Host]*RoutingRate)
	for h, rr := range latest.RuleMap {
		if current.GetRoutingRule(h) == nil {
			creates[h] = rr
		} else {
			rr.Version = current.GetRoutingRule(h).Version
			updates[h] = rr
		}
	}
	for h, rr := range current.RuleMap {
		if updates[h] == nil {
			deletes[h] = rr
		}
	}

	// TODO
	namespace := "default"
	for h, rr := range creates {
		vs := getVirtualService(mitigationRule, string(h), int(rr.InternalWeight), int(rr.ExternalWeight))
		if _, err := r.IstioClientset.NetworkingV1alpha3().VirtualServices(namespace).Create(context.TODO(), vs, metav1.CreateOptions{}); err != nil {
			return err
		}
	}
	for h, rr := range updates {
		vs := getVirtualService(mitigationRule, string(h), int(rr.InternalWeight), int(rr.ExternalWeight))
		vs.ObjectMeta.ResourceVersion = rr.Version
		if _, err := r.IstioClientset.NetworkingV1alpha3().VirtualServices(namespace).Update(context.TODO(), vs, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	for h, _ := range deletes {
		if err := r.IstioClientset.NetworkingV1alpha3().VirtualServices(namespace).Delete(context.TODO(), getVirtualServiceName(string(h)), metav1.DeleteOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func getVirtualService(mitigationRule *spikemitigationv1.MitigationRule, host string, internalWeight, externalWeight int) *v1alpha3.VirtualService {
	spec := mitigationRule.Spec
	return &v1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name: getVirtualServiceName(host),
			Labels: map[string]string{
				"InternalHost": spec.InternalHost,
				"ExternalHost": spec.ExternalHost,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: mitigationRule.APIVersion,
					Kind:       mitigationRule.Kind,
					Name:       mitigationRule.Name,
					UID:        mitigationRule.UID,
					Controller: func() *bool {
						b := true
						return &b
					}(),
				},
			},
		},
		Spec: networkingv1alpha3.VirtualService{
			Hosts: []string{host},
			// TODO external
			Gateways: []string{"greeting-gateway"},
			Http: []*networkingv1alpha3.HTTPRoute{
				&networkingv1alpha3.HTTPRoute{
					Route: []*networkingv1alpha3.HTTPRouteDestination{
						&networkingv1alpha3.HTTPRouteDestination{
							Destination: &networkingv1alpha3.Destination{
								Host: spec.ExternalHost,
								Port: &networkingv1alpha3.PortSelector{
									Number: 443,
								},
							},
							Weight: int32(externalWeight),
							Headers: &networkingv1alpha3.Headers{
								Request: &networkingv1alpha3.Headers_HeaderOperations{
									Set: map[string]string{"x-original-host": host},
								},
							},
						},
						{
							Destination: &networkingv1alpha3.Destination{
								Host: spec.InternalHost,
								Port: &networkingv1alpha3.PortSelector{
									Number: 8080,
								},
							},
							Weight: int32(internalWeight),
							Headers: &networkingv1alpha3.Headers{
								Request: &networkingv1alpha3.Headers_HeaderOperations{
									Set: map[string]string{"x-original-host": host},
								},
							},
						},
					},
					Rewrite: &networkingv1alpha3.HTTPRewrite{
						Authority: spec.ExternalHost,
					},
				},
			},
		},
	}
}

func getVirtualServiceName(host string) string {
	return fmt.Sprintf("mitigation-%s", host)
}
