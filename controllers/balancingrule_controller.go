/*


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

	"cloud.google.com/go/compute/metadata"
	"github.com/dgrijalva/jwt-go"
	"github.com/go-logr/logr"
	networkingv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	istiocli "istio.io/client-go/pkg/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubecli "k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/mmmknt/spike-mitigator/api/v1alpha1"
)

const (
	virtualServiceNamespace = "default"
	expiresBuffer           = time.Minute * 5
	reconcilePeriod         = time.Second * 30
)

// BalancingRuleReconciler reconciles a BalancingRule object
type BalancingRuleReconciler struct {
	client.Client
	KubeClientSet  *kubecli.Clientset
	IstioClientset *istiocli.Clientset
	SecretLister   corelisters.SecretLister
	Calculator     *MitigationCalculator
	Log            logr.Logger
	Scheme         *runtime.Scheme
}

type SecretValue struct {
	value   string
	version string
}

func (s *SecretValue) getVersion() string {
	if s == nil {
		return ""
	}
	return s.version
}

func (s *SecretValue) getValue() string {
	if s == nil {
		return ""
	}
	return s.value
}

// +kubebuilder:rbac:groups=loadbalancing.spike-mitigator.mmmknt.dev,resources=balancingrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=loadbalancing.spike-mitigator.mmmknt.dev,resources=balancingrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices,verbs=list;create;update;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=list;watch;create;update

func (r *BalancingRuleReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("mitigationrule", req.NamespacedName)

	log.Info("start Reconcile")
	balancingRule := &api.BalancingRule{}
	if err := r.Get(ctx, req.NamespacedName, balancingRule); err != nil {
		log.Error(err, "unable to get mitigation rule")
		return ctrl.Result{}, err
	}

	// debug
	log.Info("succeed to get mitigation rule", "mitigation rule", balancingRule)

	vsList, err := r.IstioClientset.NetworkingV1alpha3().VirtualServices(virtualServiceNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Error(err, "unable to get VirtualService list")
		return ctrl.Result{}, err
	}
	log.Info("succeed to get VirtualService List", "VirtualService", vsList)
	rm := make(map[Host]*RoutingRate)
	for i := range vsList.Items {
		item := vsList.Items[i]
		for ori := range item.OwnerReferences {
			if item.OwnerReferences[ori].Kind == "BalancingRule" {
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

	routingRule, err := r.Calculator.Calculate(ctx, r.Log, currentRR, balancingRule.Spec)
	log.Info("current", "routing rule", currentRR)
	log.Info("latest", "routing rule", routingRule)
	if err != nil {
		log.Error(err, "unable to calculate routing rule")
		return ctrl.Result{RequeueAfter: reconcilePeriod}, nil
	}
	log.Info("succeed to get routing rule", "routing rule", routingRule)

	// check and renew authorization token to CloudRun before expired
	ear := balancingRule.Spec.ExternalAuthorizationRef
	sn := balancingRule.Spec.SecretNamespace
	authorization, needUpsert, err := r.getAuthorizationToken(sn, ear.Name, ear.Key)
	if err != nil {
		log.Error(err, "unable to get authorization token from secret", "namespace", sn, "name", ear.Name, "key", ear.Key)
		return ctrl.Result{RequeueAfter: reconcilePeriod}, nil
	}
	authToken := authorization.getValue()
	authTokenVersion := authorization.getVersion()
	if needUpsert {
		serviceURL := fmt.Sprintf("https://%s", balancingRule.Spec.ExternalHost)
		tokenURL := fmt.Sprintf("/instance/service-accounts/default/identity?audience=%s", serviceURL)
		authToken, err = metadata.Get(tokenURL)
		if err != nil {
			log.Error(err, "unable to get metadata", "tokenURL", tokenURL)
			return ctrl.Result{RequeueAfter: reconcilePeriod}, nil
		}
		err = r.upsertSecret(ctx, balancingRule, sn, ear.Name, authorization.getVersion(), ear.Key, authToken)
		if err != nil {
			log.Error(err, "unable to upsert secret", "namespace", sn, "name", ear.Name, "version", authTokenVersion, "key", ear.Key)
			// want to continue processing when spike occur so not return
		}
		log.Info("succeed to upsert secret", "namespace", sn, "name", ear.Name, "key", ear.Key)
	}

	if currentRR.Equal(routingRule) {
		return ctrl.Result{RequeueAfter: 90 * time.Second}, nil
	}

	oakr := balancingRule.Spec.OptionalAuthorization.KeyRef
	oaKey, err := r.getSecretValue(sn, oakr.Name, oakr.Key)
	if err != nil {
		log.Error(err, "unable to get secret")
		return ctrl.Result{RequeueAfter: reconcilePeriod}, nil
	}
	oavr := balancingRule.Spec.OptionalAuthorization.ValueRef
	oaValue, err := r.getSecretValue(sn, oavr.Name, oavr.Key)
	if err != nil {
		log.Error(err, "unable to get secret")
		return ctrl.Result{RequeueAfter: reconcilePeriod}, nil
	}
	hihkr := balancingRule.Spec.HostInfoHeaderKeyRef
	hostInfoHeaderKey, err := r.getSecretValue(sn, hihkr.Name, hihkr.Key)
	if err != nil {
		log.Error(err, "unable to get secret")
		return ctrl.Result{RequeueAfter: reconcilePeriod}, nil
	}
	err = r.apply(balancingRule, currentRR, routingRule, hostInfoHeaderKey.value, balancingRule.Spec.GatewayName, authorization.getVersion(), oaKey.value, oaValue.value)

	// TODO make RequeueAfter to be able change per loop
	return ctrl.Result{RequeueAfter: reconcilePeriod}, err
}

func (r *BalancingRuleReconciler) getSecretValue(namespace, name, key string) (*SecretValue, error) {
	secret, err := r.SecretLister.Secrets(namespace).Get(name)
	if err != nil {
		return nil, err
	}
	bytes, ok := secret.Data[key]
	if !ok {
		return nil, fmt.Errorf("secret data is not found with namespace %s, name %s and key %s", namespace, name, key)
	}
	return &SecretValue{
		value:   string(bytes),
		version: secret.ObjectMeta.ResourceVersion,
	}, nil
}

func (r *BalancingRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.BalancingRule{}).
		Complete(r)
}

func (r *BalancingRuleReconciler) apply(mitigationRule *api.BalancingRule, current, latest *RoutingRule, hostInfoHeaderKey, gatewayName, authorization, optionalAuthorizationKey, optionalAuthorizationValue string) error {
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

	for h, rr := range creates {
		vs := getVirtualService(mitigationRule, gatewayName, string(h), hostInfoHeaderKey, authorization, optionalAuthorizationKey, optionalAuthorizationValue, int(rr.InternalWeight), int(rr.ExternalWeight))
		if _, err := r.IstioClientset.NetworkingV1alpha3().VirtualServices(virtualServiceNamespace).Create(context.TODO(), vs, metav1.CreateOptions{}); err != nil {
			return err
		}
	}
	for h, rr := range updates {
		vs := getVirtualService(mitigationRule, gatewayName, string(h), hostInfoHeaderKey, authorization, optionalAuthorizationKey, optionalAuthorizationValue, int(rr.InternalWeight), int(rr.ExternalWeight))
		vs.ObjectMeta.ResourceVersion = rr.Version
		if _, err := r.IstioClientset.NetworkingV1alpha3().VirtualServices(virtualServiceNamespace).Update(context.TODO(), vs, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	for h, _ := range deletes {
		if err := r.IstioClientset.NetworkingV1alpha3().VirtualServices(virtualServiceNamespace).Delete(context.TODO(), getVirtualServiceName(string(h)), metav1.DeleteOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func (r *BalancingRuleReconciler) upsertSecret(ctx context.Context, mitigationRule *api.BalancingRule, namespace, name, version, key, value string) error {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
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
		StringData: map[string]string{key: value},
	}

	if len(version) == 0 {
		_, err := r.KubeClientSet.CoreV1().Secrets(namespace).Create(ctx, s, metav1.CreateOptions{})
		return err
	}
	s.ObjectMeta.ResourceVersion = version
	_, err := r.KubeClientSet.CoreV1().Secrets(namespace).Update(ctx, s, metav1.UpdateOptions{})
	return err
}

func getVirtualService(mitigationRule *api.BalancingRule, gatewayName, host, hostInfoHeaderKey, authorization, optionalAuthorizationKey, optionalAuthorizationValue string, internalWeight, externalWeight int) *v1alpha3.VirtualService {
	spec := mitigationRule.Spec
	internalHeader := map[string]string{hostInfoHeaderKey: host}
	externalHeader := map[string]string{hostInfoHeaderKey: host}
	if len(authorization) > 0 {
		externalHeader["Authorization"] = fmt.Sprintf("Bearer %s", authorization)
	}
	if len(optionalAuthorizationKey) != 0 && len(optionalAuthorizationValue) != 0 {
		internalHeader[optionalAuthorizationKey] = optionalAuthorizationValue
		externalHeader[optionalAuthorizationKey] = optionalAuthorizationValue
	}
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
			Gateways: []string{gatewayName},
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
									Set: externalHeader,
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
									Set: internalHeader,
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

func (r *BalancingRuleReconciler) getAuthorizationToken(namespace, name, key string) (token *SecretValue, needUpsert bool, err error) {
	authorization, err := r.getSecretValue(namespace, name, key)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, true, nil
		} else {
			return nil, false, err
		}
	}
	exp := time.Now().Add(expiresBuffer).Unix()
	needUpsert = !r.verifyExpiresAt(authorization.value, exp)
	return authorization, needUpsert, nil
}

func (r *BalancingRuleReconciler) verifyExpiresAt(tokenString string, cmp int64) bool {
	parser := jwt.Parser{}
	claims := jwt.MapClaims{}
	// TODO fix to verify
	_, _, err := parser.ParseUnverified(tokenString, claims)
	if err != nil {
		r.Log.Error(err, "unable to parse", "token", tokenString)
		return false
	}
	return claims.VerifyExpiresAt(cmp, true)
}
