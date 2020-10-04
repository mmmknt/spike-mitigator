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
	"time"

	"github.com/go-logr/logr"
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
	if err != nil {
		log.Error(err, "unable to calculate routing rule")
		return ctrl.Result{}, err
	}
	log.Info("succeed to get routing rule", "routing rule", routingRule)
	// TODO skip if routing rule is not changed

	// TODO apply routing rule
	// TODO set labels, internal host and external host values, to VirtualService's meta

	// TODO make RequeueAfter to be able change per loop
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *MitigationRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&spikemitigationv1.MitigationRule{}).
		Complete(r)
}
